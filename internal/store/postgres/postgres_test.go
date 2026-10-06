package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
)

func TestOpen_DoesNotConnectEagerly(t *testing.T) {
	// Nothing listens on port 1: opening must still succeed, so that the server
	// starts before the database and reports it through /readyz.
	pool, err := postgres.Open(t.Context(), "postgres://magpie:secret@127.0.0.1:1/magpie?connect_timeout=1")
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.Error(t, pool.Ping(t.Context()))
}

func TestOpen_InvalidURLNeverRevealsThePassword(t *testing.T) {
	const password = "hunter2-do-not-leak"
	for _, url := range []string{
		"postgres://magpie:" + password + "@db:5432/magpie?sslmode=bogus",
		"postgres://magpie:" + password + "@db:5432/magpie?connect_timeout=never",
	} {
		_, err := postgres.Open(t.Context(), url)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), password)
		assert.Contains(t, err.Error(), "database.url")
	}
}
