//go:build integration

package cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

func TestIntegration_MigrateUpStatusDown(t *testing.T) {
	env := []string{"MAGPIE_DATABASE__URL=" + pgtest.NewDatabase(t)}

	up := run(t.Context(), env, "migrate", "up")
	require.Equal(t, 0, up.code, up.stderr)
	assert.Contains(t, up.stdout, "00001_initial_schema.sql")

	again := run(t.Context(), env, "migrate", "up")
	require.Equal(t, 0, again.code, again.stderr)
	assert.Contains(t, again.stdout, "already up to date")

	status := run(t.Context(), env, "migrate", "status")
	require.Equal(t, 0, status.code, status.stderr)
	assert.Contains(t, status.stdout, "applied")
	assert.Contains(t, status.stdout, "00001_initial_schema.sql")

	down := run(t.Context(), env, "migrate", "down")
	require.Equal(t, 0, down.code, down.stderr)
	assert.Contains(t, down.stdout, "rolled back")

	status = run(t.Context(), env, "migrate", "status")
	require.Equal(t, 0, status.code, status.stderr)
	assert.Contains(t, status.stdout, "pending")
}

func TestIntegration_RotateKeyCommand(t *testing.T) {
	url := pgtest.NewMigratedDatabase(t)
	oldKey, newKey := envelope.GenerateKey(), envelope.GenerateKey()

	parsed, err := envelope.ParseKey(oldKey)
	require.NoError(t, err)
	pool, err := postgres.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	before := postgres.NewStore(pool, envelope.NewKeyring(parsed))
	user, err := before.Users().Create(t.Context(), postgres.NewUser{Email: "rotate@example.com", Role: "admin"})
	require.NoError(t, err)
	_, err = before.Keys().ForUser(t.Context(), user.ID)
	require.NoError(t, err)

	env := []string{
		"MAGPIE_DATABASE__URL=" + url,
		"MAGPIE_SECURITY__MASTER_KEY=" + newKey,
		"MAGPIE_SECURITY__PREVIOUS_MASTER_KEYS=" + oldKey,
	}
	got := run(t.Context(), env, "admin", "rotate-key")
	require.Equal(t, 0, got.code, got.stderr)
	assert.Contains(t, got.stdout, "rewrapped 1 user key")
	for _, key := range []string{oldKey, newKey} {
		assert.NotContains(t, got.stdout+got.stderr, key, "keys are never printed")
	}

	newParsed, err := envelope.ParseKey(newKey)
	require.NoError(t, err)
	after := postgres.NewStore(pool, envelope.NewKeyring(newParsed))
	_, err = after.Keys().ForUser(t.Context(), user.ID)
	require.NoError(t, err, "the new key alone opens the user's key")

	events, err := after.Audit().List(t.Context(), postgres.AuditFilter{Limit: 5})
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, "admin.master_key_rotated", events[0].Action)
	assert.Equal(t, fmt.Sprint(int64(1)), fmt.Sprint(events[0].Details["rewrapped"]))
	assert.Equal(t, newParsed.ID(), events[0].Details["master_key_id"])

	again := run(t.Context(), env, "admin", "rotate-key")
	require.Equal(t, 0, again.code, again.stderr)
	assert.Contains(t, again.stdout, "rewrapped 0 user keys")
}
