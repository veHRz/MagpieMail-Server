//go:build integration

package cli_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

// TestIntegration_ReadinessFollowsTheDatabase runs `magpie serve` against a real
// PostgreSQL: /readyz answers 200 while the database is up, 503 once it is gone.
func TestIntegration_ReadinessFollowsTheDatabase(t *testing.T) {
	ctr, url := pgtest.Start(t)
	addr := freeAddr(t)
	env := []string{"MAGPIE_DATABASE__URL=" + url, "MAGPIE_SERVER__LISTEN=" + addr}

	ctx, stop := context.WithCancel(t.Context())
	results := make(chan result, 1)
	go func() { results <- run(ctx, env, "serve") }()

	readyz := func() int {
		status, err := getStatus(t.Context(), "http://"+addr+"/readyz")
		if err != nil {
			return 0
		}
		return status
	}

	require.Eventually(t, func() bool { return readyz() == http.StatusOK },
		10*time.Second, 50*time.Millisecond, "/readyz never answered 200")
	assert.Equal(t, 0, run(t.Context(), env, "healthcheck").code)

	require.NoError(t, ctr.Stop(t.Context(), nil))
	require.Eventually(t, func() bool { return readyz() == http.StatusServiceUnavailable },
		10*time.Second, 50*time.Millisecond, "/readyz kept answering after the database stopped")
	assert.Equal(t, 1, run(t.Context(), env, "healthcheck").code)

	stop()
	select {
	case got := <-results:
		assert.Equal(t, 0, got.code, "stderr: %s", got.stderr)
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}
