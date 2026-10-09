//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

// mvpTables is the data model of the MVP, as listed by phase S2 of the plan,
// plus user_keys and global_folder_messages.
var mvpTables = []string{
	"users", "user_keys", "sessions", "credentials", "policies", "providers",
	"accounts", "identities", "sync_policies", "mailboxes", "messages",
	"message_locations", "threads", "tags", "message_tags", "global_folders",
	"global_folder_messages", "attachments", "blobs", "drafts", "outbox", "audit_log",
}

func tables(t *testing.T, url string) []string {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), url)
	require.NoError(t, err)
	defer conn.Close(context.WithoutCancel(t.Context()))
	rows, err := conn.Query(t.Context(),
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name")
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return names
}

// TestIntegration_MigrationsGoUpAndDownOnAnEmptyDatabase proves the S2
// criterion "every migration goes up and down on an empty database".
func TestIntegration_MigrationsGoUpAndDownOnAnEmptyDatabase(t *testing.T) {
	url := pgtest.NewDatabase(t)
	m, err := postgres.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	up, err := m.Up(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, up)
	for _, table := range mvpTables {
		assert.Contains(t, tables(t, url), table)
	}

	status, err := m.Status(t.Context())
	require.NoError(t, err)
	for _, s := range status {
		assert.True(t, s.Applied, "migration %d %s is not applied", s.Version, s.Name)
	}

	down, err := m.DownAll(t.Context())
	require.NoError(t, err)
	assert.Len(t, down, len(up), "every migration goes down")
	assert.Equal(t, []string{"goose_db_version"}, tables(t, url), "nothing but goose's bookkeeping is left")

	again, err := m.Up(t.Context())
	require.NoError(t, err)
	assert.Len(t, again, len(up), "and up again")
}

func TestIntegration_MigratorReportsPendingMigrations(t *testing.T) {
	m, err := postgres.NewMigrator(pgtest.NewDatabase(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	status, err := m.Status(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, status)
	for _, s := range status {
		assert.False(t, s.Applied)
	}

	one, err := m.Down(t.Context())
	require.Error(t, err, "nothing to roll back on an empty database")
	assert.Nil(t, one)
}

func TestNewMigrator_InvalidURLNeverRevealsThePassword(t *testing.T) {
	_, err := postgres.NewMigrator("postgres://magpie:hunter2-secret@db:5432/magpie?sslmode=bogus")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2-secret")
}
