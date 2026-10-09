//go:build integration

package postgres_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

func masterKey(t *testing.T) envelope.Key {
	t.Helper()
	k, err := envelope.ParseKey(envelope.GenerateKey())
	require.NoError(t, err)
	return k
}

// newStore returns a store on a fresh, migrated database, and its URL.
func newStore(t *testing.T, ring *envelope.Keyring) (*postgres.Store, string) {
	t.Helper()
	url := pgtest.NewMigratedDatabase(t)
	pool, err := postgres.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return postgres.NewStore(pool, ring), url
}

func reopen(t *testing.T, url string, ring *envelope.Keyring) *postgres.Store {
	t.Helper()
	pool, err := postgres.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return postgres.NewStore(pool, ring)
}

func createUser(t *testing.T, s *postgres.Store, email string) postgres.User {
	t.Helper()
	u, err := s.Users().Create(t.Context(), postgres.NewUser{Email: email, DisplayName: "Test", Role: "user"})
	require.NoError(t, err)
	return u
}

func createProvider(t *testing.T, s *postgres.Store) postgres.Provider {
	t.Helper()
	p, err := s.Providers().Create(t.Context(), postgres.NewProvider{
		Slug: "custom-" + uuid.NewString()[:8], Name: "Custom", Kind: "custom",
		Settings: map[string]any{"imap": map[string]any{"host": "imap.example.com", "port": 993}},
	})
	require.NoError(t, err)
	return p
}

func createAccount(t *testing.T, s *postgres.Store, user postgres.User, credentials string) postgres.Account {
	t.Helper()
	a, err := s.Accounts().Create(t.Context(), postgres.NewAccount{
		UserID: user.ID, ProviderID: createProvider(t, s).ID,
		Email: "box-" + uuid.NewString()[:8] + "@example.com", AuthMethod: "password",
		Credentials: []byte(credentials),
	})
	require.NoError(t, err)
	return a
}

func TestIntegration_Users(t *testing.T) {
	s, _ := newStore(t, nil)

	created := createUser(t, s, "Alice@Example.com")
	assert.Equal(t, uuid.Version(7), created.ID.Version())
	assert.Equal(t, "user", created.Role)
	assert.Equal(t, time.UTC, created.CreatedAt.Location())

	got, err := s.Users().Get(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Email, got.Email)

	byEmail, err := s.Users().GetByEmail(t.Context(), "alice@example.COM")
	require.NoError(t, err, "emails match case-insensitively")
	assert.Equal(t, created.ID, byEmail.ID)

	_, err = s.Users().Create(t.Context(), postgres.NewUser{Email: "ALICE@example.com", Role: "user"})
	require.ErrorIs(t, err, postgres.ErrConflict)

	_, err = s.Users().Get(t.Context(), uuid.New())
	require.ErrorIs(t, err, postgres.ErrNotFound)
	_, err = s.Users().GetByEmail(t.Context(), "nobody@example.com")
	require.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestIntegration_UserKeysAreCreatedOnceAndReused(t *testing.T) {
	s, _ := newStore(t, envelope.NewKeyring(masterKey(t)))
	user := createUser(t, s, "keys@example.com")

	first, err := s.Keys().ForUser(t.Context(), user.ID)
	require.NoError(t, err)
	sealed, err := first.Seal([]byte("x"), nil)
	require.NoError(t, err)

	second, err := s.Keys().ForUser(t.Context(), user.ID)
	require.NoError(t, err)
	opened, err := second.Open(sealed, nil)
	require.NoError(t, err, "the same key comes back")
	assert.Equal(t, []byte("x"), opened)

	_, err = s.Keys().ForUser(t.Context(), uuid.New())
	require.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestIntegration_StoresWithoutAKeyringRefuseSecrets(t *testing.T) {
	s, _ := newStore(t, nil)
	user := createUser(t, s, "nokeys@example.com")

	_, err := s.Keys().ForUser(t.Context(), user.ID)
	require.ErrorIs(t, err, postgres.ErrNoKeyring)
	_, err = s.RotateMasterKey(t.Context())
	require.ErrorIs(t, err, postgres.ErrNoKeyring)
}

func TestIntegration_AccountCredentialsAreSealedPerAccount(t *testing.T) {
	s, _ := newStore(t, envelope.NewKeyring(masterKey(t)))
	alice := createUser(t, s, "alice@example.com")
	bob := createUser(t, s, "bob@example.com")
	account := createAccount(t, s, alice, "app-password-1")

	got, err := s.Accounts().Credentials(t.Context(), alice.ID, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "app-password-1", string(got))

	require.NoError(t, s.Accounts().SetCredentials(t.Context(), alice.ID, account.ID, []byte("refresh-token-2")))
	got, err = s.Accounts().Credentials(t.Context(), alice.ID, account.ID)
	require.NoError(t, err)
	assert.Equal(t, "refresh-token-2", string(got))

	fetched, err := s.Accounts().Get(t.Context(), alice.ID, account.ID)
	require.NoError(t, err)
	assert.Equal(t, account.Email, fetched.Email)

	// Another user sees nothing, even with the right account ID.
	_, err = s.Accounts().Get(t.Context(), bob.ID, account.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound)
	_, err = s.Accounts().Credentials(t.Context(), bob.ID, account.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound)
	require.ErrorIs(t, s.Accounts().SetCredentials(t.Context(), bob.ID, account.ID, []byte("x")), postgres.ErrNotFound)

	_, err = s.Accounts().Create(t.Context(), postgres.NewAccount{
		UserID: alice.ID, ProviderID: uuid.New(), Email: "x@example.com", AuthMethod: "password",
	})
	require.ErrorIs(t, err, postgres.ErrNotFound, "unknown provider")
}

func TestIntegration_CiphertextMovedToAnotherAccountDoesNotOpen(t *testing.T) {
	s, url := newStore(t, envelope.NewKeyring(masterKey(t)))
	alice := createUser(t, s, "alice@example.com")
	first := createAccount(t, s, alice, "first-secret")
	second := createAccount(t, s, alice, "second-secret")

	conn, err := pgx.Connect(t.Context(), url)
	require.NoError(t, err)
	defer conn.Close(context.WithoutCancel(t.Context()))
	_, err = conn.Exec(t.Context(),
		`UPDATE accounts SET credentials_encrypted = (SELECT credentials_encrypted FROM accounts WHERE id = $1) WHERE id = $2`,
		first.ID, second.ID)
	require.NoError(t, err)

	_, err = s.Accounts().Credentials(t.Context(), alice.ID, second.ID)
	require.ErrorIs(t, err, envelope.ErrDecrypt)
}

// TestIntegration_NoSecretIsReadableInRawColumns proves the S2 criterion "no
// secret can be read in clear text in the raw columns": it searches every
// column of every table for the secret, as text and as bytes.
func TestIntegration_NoSecretIsReadableInRawColumns(t *testing.T) {
	const secret = "imap-app-password-must-not-leak"
	s, url := newStore(t, envelope.NewKeyring(masterKey(t)))
	user := createUser(t, s, "secret-holder@example.com")
	account := createAccount(t, s, user, secret)

	got, err := s.Accounts().Credentials(t.Context(), user.ID, account.ID)
	require.NoError(t, err)
	require.Equal(t, secret, string(got), "the secret is stored and readable through the repository")

	conn, err := pgx.Connect(t.Context(), url)
	require.NoError(t, err)
	defer conn.Close(context.WithoutCancel(t.Context()))
	rows, err := conn.Query(t.Context(), `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public' ORDER BY table_name, column_name`)
	require.NoError(t, err)
	type column struct{ table, name string }
	columns, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (column, error) {
		var c column
		return c, r.Scan(&c.table, &c.name)
	})
	require.NoError(t, err)
	require.NotEmpty(t, columns)

	hexSecret := hex.EncodeToString([]byte(secret))
	for _, c := range columns {
		var hits int
		query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s::text ILIKE '%%' || $1 || '%%' OR %s::text ILIKE '%%' || $2 || '%%'`,
			pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.name}.Sanitize(), pgx.Identifier{c.name}.Sanitize())
		require.NoError(t, conn.QueryRow(t.Context(), query, secret, hexSecret).Scan(&hits))
		assert.Zero(t, hits, "%s.%s contains the secret in clear", c.table, c.name)
	}
}

// TestIntegration_MasterKeyRotationLosesNothing proves the S2 criterion "the
// master key rotation re-encrypts everything without loss".
func TestIntegration_MasterKeyRotationLosesNothing(t *testing.T) {
	oldKey, newKey := masterKey(t), masterKey(t)
	before, url := newStore(t, envelope.NewKeyring(oldKey))

	type stored struct {
		user    postgres.User
		account postgres.Account
		secret  string
	}
	var data []stored
	for i := range 5 {
		u := createUser(t, before, fmt.Sprintf("user%d@example.com", i))
		secret := fmt.Sprintf("secret-%d-%s", i, uuid.NewString())
		data = append(data, stored{u, createAccount(t, before, u, secret), secret})
	}
	counts, err := before.Keys().CountByMasterKey(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{oldKey.ID(): 5}, counts)

	// Step 1: the new key is current, the old one still known.
	during := reopen(t, url, envelope.NewKeyring(newKey, oldKey))
	rewrapped, err := during.RotateMasterKey(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 5, rewrapped)

	counts, err = during.Keys().CountByMasterKey(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{newKey.ID(): 5}, counts, "every user key moved to the new master key")

	again, err := during.RotateMasterKey(t.Context())
	require.NoError(t, err)
	assert.Zero(t, again, "a second rotation has nothing to do")

	// Step 2: the old key is gone; everything still opens.
	after := reopen(t, url, envelope.NewKeyring(newKey))
	for _, d := range data {
		got, err := after.Accounts().Credentials(t.Context(), d.user.ID, d.account.ID)
		require.NoError(t, err)
		assert.Equal(t, d.secret, string(got))
	}

	// And the old key alone opens nothing any more.
	stale := reopen(t, url, envelope.NewKeyring(oldKey))
	_, err = stale.Accounts().Credentials(t.Context(), data[0].user.ID, data[0].account.ID)
	require.ErrorIs(t, err, envelope.ErrUnknownKey)
}

func TestIntegration_RotationRefusesKeysItCannotOpen(t *testing.T) {
	s, url := newStore(t, envelope.NewKeyring(masterKey(t)))
	user := createUser(t, s, "lost@example.com")
	createAccount(t, s, user, "secret")

	unrelated := reopen(t, url, envelope.NewKeyring(masterKey(t)))
	_, err := unrelated.RotateMasterKey(t.Context())
	require.ErrorIs(t, err, envelope.ErrUnknownKey, "without the previous key, nothing is rewritten")

	counts, err := s.Keys().CountByMasterKey(t.Context())
	require.NoError(t, err)
	assert.Len(t, counts, 1, "the failed rotation rolled back")
}

func TestIntegration_BlobRows(t *testing.T) {
	s, _ := newStore(t, nil)
	alice := createUser(t, s, "alice@example.com")
	bob := createUser(t, s, "bob@example.com")
	fingerprint := []byte("0123456789abcdef0123456789abcdef")

	blob, inserted, err := s.Blobs().Upsert(t.Context(), alice.ID, fingerprint, 42, true)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Zero(t, blob.RefCount)

	same, inserted, err := s.Blobs().Upsert(t.Context(), alice.ID, fingerprint, 42, true)
	require.NoError(t, err)
	assert.False(t, inserted, "same content, same user: one blob")
	assert.Equal(t, blob.ID, same.ID)

	other, inserted, err := s.Blobs().Upsert(t.Context(), bob.ID, fingerprint, 42, true)
	require.NoError(t, err)
	assert.True(t, inserted, "never shared across users")
	assert.NotEqual(t, blob.ID, other.ID)

	refs, err := s.Blobs().AddRef(t.Context(), alice.ID, blob.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), refs)
	refs, err = s.Blobs().Release(t.Context(), alice.ID, blob.ID)
	require.NoError(t, err)
	assert.Zero(t, refs)
	_, err = s.Blobs().Release(t.Context(), alice.ID, blob.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound, "no reference left to release")

	_, err = s.Blobs().AddRef(t.Context(), bob.ID, blob.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound, "another user's blob")
	_, err = s.Blobs().Get(t.Context(), bob.ID, blob.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound)
	got, err := s.Blobs().Get(t.Context(), alice.ID, blob.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.Size)
}

func TestIntegration_PurgeableBlobsAreLockedAndDeleted(t *testing.T) {
	s, _ := newStore(t, nil)
	user := createUser(t, s, "purge@example.com")
	kept, _, err := s.Blobs().Upsert(t.Context(), user.ID, []byte("kept-kept-kept-kept-kept-kept-kp"), 1, false)
	require.NoError(t, err)
	_, err = s.Blobs().AddRef(t.Context(), user.ID, kept.ID)
	require.NoError(t, err)
	orphan, _, err := s.Blobs().Upsert(t.Context(), user.ID, []byte("orphan-orphan-orphan-orphan-orph"), 1, false)
	require.NoError(t, err)

	err = s.InTx(t.Context(), func(tx *postgres.Store) error {
		purgeable, err := tx.Blobs().LockPurgeable(t.Context(), time.Now().Add(time.Minute), 10)
		require.NoError(t, err)
		require.Len(t, purgeable, 1)
		assert.Equal(t, orphan.ID, purgeable[0].ID)
		return tx.Blobs().Delete(t.Context(), orphan.ID)
	})
	require.NoError(t, err)

	_, err = s.Blobs().Get(t.Context(), user.ID, orphan.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound)
	_, err = s.Blobs().Get(t.Context(), user.ID, kept.ID)
	require.NoError(t, err)

	none, err := s.Blobs().LockPurgeable(t.Context(), time.Now().Add(-time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, none, "recent orphans are within the grace period")
}

func TestIntegration_TransactionsRollBackOnError(t *testing.T) {
	s, _ := newStore(t, nil)
	boom := fmt.Errorf("boom")

	err := s.InTx(t.Context(), func(tx *postgres.Store) error {
		_, err := tx.Users().Create(t.Context(), postgres.NewUser{Email: "rolled@example.com", Role: "user"})
		require.NoError(t, err)
		return boom
	})
	require.ErrorIs(t, err, boom)

	_, err = s.Users().GetByEmail(t.Context(), "rolled@example.com")
	require.ErrorIs(t, err, postgres.ErrNotFound)

	err = s.InTx(t.Context(), func(tx *postgres.Store) error {
		return tx.InTx(t.Context(), func(*postgres.Store) error { return nil })
	})
	require.ErrorIs(t, err, postgres.ErrNestedTx)
}

func TestIntegration_AuditLog(t *testing.T) {
	s, _ := newStore(t, nil)
	alice := createUser(t, s, "alice@example.com")
	ip := netip.MustParseAddr("192.0.2.7")
	base := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)

	for i, action := range []string{"auth.login", "auth.login_failed", "admin.user_created"} {
		actor := &alice.ID
		if action == "admin.user_created" {
			actor = nil
		}
		require.NoError(t, s.Audit().Append(t.Context(), postgres.AuditEvent{
			OccurredAt: base.Add(time.Duration(i) * time.Minute), ActorUserID: actor, Action: action,
			TargetType: "user", TargetID: alice.ID.String(), ClientIP: &ip,
			Details: map[string]any{"method": "password"},
		}))
	}

	all, err := s.Audit().List(t.Context(), postgres.AuditFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "admin.user_created", all[0].Action, "newest first")
	assert.Equal(t, uuid.Version(7), all[0].ID.Version())
	assert.Equal(t, ip, *all[1].ClientIP)
	assert.Equal(t, map[string]any{"method": "password"}, all[1].Details)

	mine, err := s.Audit().List(t.Context(), postgres.AuditFilter{Actor: &alice.ID, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, mine, 2)

	older, err := s.Audit().List(t.Context(), postgres.AuditFilter{Before: base.Add(90 * time.Second), Limit: 1})
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, "auth.login_failed", older[0].Action, "paginates backwards in time")

	now := time.Now()
	require.NoError(t, s.Audit().Append(t.Context(), postgres.AuditEvent{Action: "system.started"}))
	latest, err := s.Audit().List(t.Context(), postgres.AuditFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, latest, 1)
	assert.WithinDuration(t, now, latest[0].OccurredAt, time.Minute, "occurred_at defaults to now")
	assert.Empty(t, latest[0].Details)
}
