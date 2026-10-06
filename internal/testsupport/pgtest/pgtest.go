//go:build integration

// Package pgtest starts throwaway PostgreSQL servers for integration tests.
package pgtest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image is the PostgreSQL image of integration tests. It must match the image
// of deploy/docker-compose.yml, which TestImageMatchesCompose checks.
const Image = "postgres:18.6-alpine"

// Start runs a PostgreSQL container for the duration of the test and returns
// it with a connection URL.
func Start(t testing.TB) (*tcpostgres.PostgresContainer, string) {
	t.Helper()
	ctr, err := tcpostgres.Run(t.Context(), Image,
		tcpostgres.WithDatabase("magpie"),
		tcpostgres.WithUsername("magpie"),
		tcpostgres.WithPassword("magpie"),
		tcpostgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	url, err := ctr.ConnectionString(t.Context(), "sslmode=disable")
	require.NoError(t, err)
	return ctr, url
}
