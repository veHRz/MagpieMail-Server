package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
)

// masterKeyEnv returns a fresh master key as an environment entry.
func masterKeyEnv() string {
	return "MAGPIE_SECURITY__MASTER_KEY=" + envelope.GenerateKey()
}

func TestAdmin_WithoutSubcommandPrintsUsage(t *testing.T) {
	got := run(t.Context(), nil, "admin")

	assert.Equal(t, 0, got.code)
	assert.Contains(t, got.stdout, "generate-key")
	assert.Contains(t, got.stdout, "rotate-key")
}

func TestAdmin_GenerateKeyPrintsANewMasterKey(t *testing.T) {
	first := run(t.Context(), nil, "admin", "generate-key")
	second := run(t.Context(), nil, "admin", "generate-key")

	require.Equal(t, 0, first.code, first.stderr)
	_, err := envelope.ParseKey(first.stdout)
	require.NoError(t, err, "the output is a usable master key")
	assert.Equal(t, 1, strings.Count(first.stdout, "\n"), "only the key, so that it can be piped to a file")
	assert.NotEqual(t, first.stdout, second.stdout)
}

func TestAdmin_RotateKeyRequiresAMasterKey(t *testing.T) {
	got := run(t.Context(), []string{unreachableDB}, "admin", "rotate-key")

	assert.Equal(t, 1, got.code)
	assert.Contains(t, got.stderr, "security.master_key")
	assert.Contains(t, got.stderr, "generate-key")
}

func TestServe_RequiresAMasterKey(t *testing.T) {
	got := run(t.Context(), []string{unreachableDB}, "serve")

	assert.Equal(t, 1, got.code)
	assert.Contains(t, got.stderr, "security.master_key")
}

func TestMigrate_WithoutSubcommandPrintsUsage(t *testing.T) {
	got := run(t.Context(), nil, "migrate")

	assert.Equal(t, 0, got.code)
	for _, sub := range []string{"up", "down", "status"} {
		assert.Contains(t, got.stdout, sub)
	}
}

func TestMigrate_FailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	got := run(t.Context(), []string{unreachableDB}, "migrate", "up")

	assert.Equal(t, 1, got.code)
	assert.NotEmpty(t, got.stderr)
	assert.NotContains(t, got.stderr, "secret", "the database password never appears")
}
