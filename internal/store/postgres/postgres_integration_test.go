//go:build integration

package postgres_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

func TestIntegration_OpenConnectsToPostgreSQL(t *testing.T) {
	_, url := pgtest.Start(t)

	pool, err := postgres.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(t.Context()))

	var version, application string
	require.NoError(t, pool.QueryRow(t.Context(),
		"SELECT current_setting('server_version'), current_setting('application_name')").Scan(&version, &application))
	assert.True(t, strings.HasPrefix(version, "18."), "unexpected PostgreSQL version %s", version)
	assert.Equal(t, "magpie", application)
}
