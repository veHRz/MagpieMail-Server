//go:build integration

package pgtest_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

func TestImageMatchesCompose(t *testing.T) {
	compose, err := os.ReadFile("../../../deploy/docker-compose.yml")
	require.NoError(t, err)

	assert.Contains(t, string(compose), "image: "+pgtest.Image+"@sha256:",
		"integration tests and the compose stack must run the same PostgreSQL")
}
