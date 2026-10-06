package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi"
)

// startServer runs Serve in the background and returns its base URL, a function
// that stops it, and a channel that receives Serve's result.
func startServer(t *testing.T, handler http.Handler) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		done <- httpapi.Serve(ctx, ln, handler, serverConfig(), slog.New(slog.DiscardHandler))
	}()
	return "http://" + ln.Addr().String(), cancel, done
}

func waitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after shutdown")
		return nil
	}
}

func getStatus(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestServe_ServesUntilTheContextIsCancelled(t *testing.T) {
	router, _ := newRouter(t, fakeDB{})
	baseURL, stop, done := startServer(t, router)

	assert.Equal(t, http.StatusOK, getStatus(t, baseURL+"/healthz"))

	stop()
	require.NoError(t, waitResult(t, done))
}

func TestServe_LetsInFlightRequestsFinishOnShutdown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})
	baseURL, stop, done := startServer(t, slow)

	type result struct {
		status int
		body   string
		err    error
	}
	responses := make(chan result, 1)
	go func() {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL, http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			responses <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		responses <- result{status: resp.StatusCode, body: string(body), err: err}
	}()

	<-started
	stop()
	time.Sleep(50 * time.Millisecond) // let Shutdown begin while the request is in flight
	close(release)

	got := <-responses
	require.NoError(t, got.err)
	assert.Equal(t, http.StatusOK, got.status)
	assert.Equal(t, "finished", got.body)
	require.NoError(t, waitResult(t, done))
}
