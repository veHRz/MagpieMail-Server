package cli_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/cli"
)

// unreachableDB points at a port where nothing listens: the server must still
// start, and report the database through /readyz.
const unreachableDB = "MAGPIE_DATABASE__URL=postgres://magpie:secret@127.0.0.1:1/magpie?connect_timeout=1"

type result struct {
	code   int
	stdout string
	stderr string
}

func run(ctx context.Context, env []string, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := cli.Run(ctx, args, env, &stdout, &stderr)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// freeAddr returns a loopback address with a port that was free a moment ago.
func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func getStatus(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func TestRun_WithoutArgumentsPrintsUsage(t *testing.T) {
	got := run(t.Context(), nil)

	assert.Equal(t, 0, got.code)
	for _, sub := range []string{"serve", "worker", "migrate", "admin"} {
		assert.Contains(t, got.stdout, sub)
	}
	assert.NotContains(t, got.stdout, "healthcheck", "the container probe is hidden from the help")
}

func TestRun_PrintsVersion(t *testing.T) {
	got := run(t.Context(), nil, "--version")

	assert.Equal(t, 0, got.code)
	assert.Contains(t, got.stdout, "magpie version")
}

func TestRun_UnknownCommandFails(t *testing.T) {
	got := run(t.Context(), nil, "frobnicate")

	assert.Equal(t, 1, got.code)
	assert.Contains(t, got.stderr, "frobnicate")
}

func TestRun_PlaceholderCommandsAreNotImplemented(t *testing.T) {
	for _, sub := range []string{"worker", "migrate", "admin"} {
		t.Run(sub, func(t *testing.T) {
			got := run(t.Context(), nil, sub)

			assert.Equal(t, 1, got.code)
			assert.Contains(t, got.stderr, "not implemented yet")
		})
	}
}

// TestServe_InvalidConfigurationFailsNamingTheKey proves the S0 criterion
// "an invalid configuration fails startup with a message naming the faulty key"
// at the command level, for both configuration sources.
func TestServe_InvalidConfigurationFailsNamingTheKey(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		got := run(t.Context(), []string{unreachableDB, "MAGPIE_SERVER__LISTEN=nope"}, "serve")

		assert.Equal(t, 1, got.code)
		assert.Contains(t, got.stderr, "server.listen")
		assert.Contains(t, got.stderr, "MAGPIE_SERVER__LISTEN")
	})

	t.Run("file from flag", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "magpie.yaml")
		require.NoError(t, os.WriteFile(path, []byte("log:\n  lvl: debug\n"), 0o600))

		got := run(t.Context(), []string{unreachableDB}, "serve", "--config", path)

		assert.Equal(t, 1, got.code)
		assert.Contains(t, got.stderr, "log.lvl")
		assert.Contains(t, got.stderr, path)
	})

	t.Run("file from MAGPIE_CONFIG", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "magpie.yaml")
		require.NoError(t, os.WriteFile(path, []byte("server:\n  read_timeout: 30\n"), 0o600))

		got := run(t.Context(), []string{unreachableDB, "MAGPIE_CONFIG=" + path}, "serve")

		assert.Equal(t, 1, got.code)
		assert.Contains(t, got.stderr, "server.read_timeout")
	})
}

func TestServe_StartsWithoutTheDatabaseAndStopsCleanly(t *testing.T) {
	addr := freeAddr(t)
	ctx, stop := context.WithCancel(t.Context())
	results := make(chan result, 1)
	go func() {
		results <- run(ctx, []string{unreachableDB, "MAGPIE_SERVER__LISTEN=" + addr}, "serve")
	}()

	var healthz int
	require.Eventually(t, func() bool {
		status, err := getStatus(t.Context(), "http://"+addr+"/healthz")
		healthz = status
		return err == nil
	}, 5*time.Second, 20*time.Millisecond, "server did not start")
	assert.Equal(t, http.StatusOK, healthz)

	readyz, err := getStatus(t.Context(), "http://"+addr+"/readyz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, readyz)

	stop()
	select {
	case got := <-results:
		assert.Equal(t, 0, got.code, "stderr: %s", got.stderr)
		assert.NotContains(t, got.stderr, "secret", "the database password must never be logged")
		for line := range strings.Lines(got.stderr) {
			assert.True(t, strings.HasPrefix(line, "{"), "log line is not JSON: %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}

func TestHealthcheck_ReflectsReadiness(t *testing.T) {
	tests := map[string]struct {
		status int
		want   int
	}{
		"ready":     {status: http.StatusOK, want: 0},
		"not ready": {status: http.StatusServiceUnavailable, want: 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			probed := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				probed <- r.URL.Path
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(srv.Close)

			// The server listens on all interfaces; the probe must target loopback.
			_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
			require.NoError(t, err)
			got := run(t.Context(), []string{unreachableDB, "MAGPIE_SERVER__LISTEN=:" + port}, "healthcheck")

			assert.Equal(t, tt.want, got.code, "stderr: %s", got.stderr)
			assert.Equal(t, "/readyz", <-probed)
		})
	}
}

func TestHealthcheck_FailsWhenNothingListens(t *testing.T) {
	got := run(t.Context(), []string{unreachableDB, "MAGPIE_SERVER__LISTEN=" + freeAddr(t)}, "healthcheck")

	assert.Equal(t, 1, got.code)
	assert.NotEmpty(t, got.stderr)
}
