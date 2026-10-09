//go:build integration

package blobs_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/blobs"
	"github.com/veHRz/MagpieMail-Server/internal/store/blobstore"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
	"github.com/veHRz/MagpieMail-Server/internal/testsupport/pgtest"
)

type env struct {
	store   *postgres.Store
	service *blobs.Service
	root    string
	clock   *time.Time
}

func newEnv(t *testing.T, encrypt bool) env {
	t.Helper()
	key, err := envelope.ParseKey(envelope.GenerateKey())
	require.NoError(t, err)
	pool, err := postgres.Open(t.Context(), pgtest.NewMigratedDatabase(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	store := postgres.NewStore(pool, envelope.NewKeyring(key))

	root := filepath.Join(t.TempDir(), "blobs")
	disk, err := blobstore.NewDisk(root)
	require.NoError(t, err)
	clock := time.Now()
	return env{
		store: store,
		service: blobs.NewService(store, disk, blobs.Options{
			Encrypt: encrypt, Grace: time.Hour, Now: func() time.Time { return clock },
		}),
		root:  root,
		clock: &clock,
	}
}

func (e env) user(t *testing.T) uuid.UUID {
	t.Helper()
	u, err := e.store.Users().Create(t.Context(), postgres.NewUser{Email: uuid.NewString() + "@example.com", Role: "user"})
	require.NoError(t, err)
	return u.ID
}

// objects lists the files of the disk store, temporary directory excluded.
func (e env) objects(t *testing.T) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(e.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "tmp" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	}))
	return files
}

func (e env) read(t *testing.T, userID, blobID uuid.UUID) string {
	t.Helper()
	r, err := e.service.Open(t.Context(), userID, blobID)
	require.NoError(t, err)
	defer r.Close()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}

// TestIntegration_SameContentTwiceIsStoredOnce proves the S2 criterion
// "writing the same file twice creates a single blob".
func TestIntegration_SameContentTwiceIsStoredOnce(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)
	content := strings.Repeat("Invoice #42, due 2026-11-01. ", 1000)

	first, err := e.service.Write(t.Context(), user, strings.NewReader(content))
	require.NoError(t, err)
	second, err := e.service.Write(t.Context(), user, strings.NewReader(content))
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "one blob record")
	assert.Len(t, e.objects(t), 1, "one stored object")
	assert.Equal(t, int64(len(content)), first.Size)
	assert.Equal(t, content, e.read(t, user, first.ID))
}

func TestIntegration_UsersNeverShareBlobs(t *testing.T) {
	e := newEnv(t, true)
	alice, bob := e.user(t), e.user(t)

	a, err := e.service.Write(t.Context(), alice, strings.NewReader("same content"))
	require.NoError(t, err)
	b, err := e.service.Write(t.Context(), bob, strings.NewReader("same content"))
	require.NoError(t, err)

	assert.NotEqual(t, a.ID, b.ID)
	assert.NotEqual(t, a.Fingerprint, b.Fingerprint, "fingerprints are keyed per user")
	assert.Len(t, e.objects(t), 2)

	_, err = e.service.Open(t.Context(), bob, a.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound, "bob cannot open alice's blob")
}

func TestIntegration_EncryptedBlobsAreUnreadableOnDisk(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)
	content := "Dear Alice, the meeting is moved to Friday."

	blob, err := e.service.Write(t.Context(), user, strings.NewReader(content))
	require.NoError(t, err)
	assert.True(t, blob.Encrypted)

	files := e.objects(t)
	require.Len(t, files, 1)
	onDisk, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), "meeting")
	assert.Equal(t, content, e.read(t, user, blob.ID))
}

func TestIntegration_PlainBlobsWhenEncryptionIsOff(t *testing.T) {
	e := newEnv(t, false)
	user := e.user(t)

	blob, err := e.service.Write(t.Context(), user, strings.NewReader("plain content"))
	require.NoError(t, err)
	assert.False(t, blob.Encrypted)

	files := e.objects(t)
	require.Len(t, files, 1)
	onDisk, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Equal(t, "plain content", string(onDisk))
	assert.Equal(t, "plain content", e.read(t, user, blob.ID))
}

// TestIntegration_OrphanBlobDisappearsAtPurge proves the S2 criterion "an
// orphan blob disappears at the purge", once its grace period is over.
func TestIntegration_OrphanBlobDisappearsAtPurge(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)
	orphan, err := e.service.Write(t.Context(), user, strings.NewReader("nobody references me"))
	require.NoError(t, err)
	kept, err := e.service.Write(t.Context(), user, strings.NewReader("a message references me"))
	require.NoError(t, err)
	_, err = e.store.Blobs().AddRef(t.Context(), user, kept.ID)
	require.NoError(t, err)

	report, err := e.service.Purge(t.Context())
	require.NoError(t, err)
	assert.Zero(t, report.Deleted, "nothing is purged within the grace period")
	assert.Len(t, e.objects(t), 2)

	*e.clock = e.clock.Add(2 * time.Hour)
	report, err = e.service.Purge(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, report.Deleted)
	assert.Equal(t, orphan.Size, report.Bytes)

	_, err = e.store.Blobs().Get(t.Context(), user, orphan.ID)
	require.ErrorIs(t, err, postgres.ErrNotFound, "the record is gone")
	assert.Len(t, e.objects(t), 1, "the object is gone")
	assert.Equal(t, "a message references me", e.read(t, user, kept.ID), "the referenced blob stays")
}

func TestIntegration_ReleasedBlobIsPurged(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)
	blob, err := e.service.Write(t.Context(), user, strings.NewReader("short-lived"))
	require.NoError(t, err)
	_, err = e.store.Blobs().AddRef(t.Context(), user, blob.ID)
	require.NoError(t, err)
	_, err = e.store.Blobs().Release(t.Context(), user, blob.ID)
	require.NoError(t, err)

	*e.clock = e.clock.Add(2 * time.Hour)
	report, err := e.service.Purge(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, report.Deleted)
	assert.Empty(t, e.objects(t))
}

func TestIntegration_MissingObjectIsRewrittenOnTheNextWrite(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)
	blob, err := e.service.Write(t.Context(), user, strings.NewReader("fragile"))
	require.NoError(t, err)
	for _, f := range e.objects(t) {
		require.NoError(t, os.Remove(f))
	}

	_, err = e.service.Open(t.Context(), user, blob.ID)
	require.ErrorIs(t, err, blobstore.ErrNotFound)

	again, err := e.service.Write(t.Context(), user, strings.NewReader("fragile"))
	require.NoError(t, err)
	assert.Equal(t, blob.ID, again.ID)
	assert.Equal(t, "fragile", e.read(t, user, blob.ID))
}

func TestIntegration_FailedWriteRecordsNothing(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)

	_, err := e.service.Write(t.Context(), user, io.MultiReader(strings.NewReader("half"), errReader{}))
	require.Error(t, err)

	assert.Empty(t, e.objects(t))
	report, err := e.service.Purge(t.Context())
	require.NoError(t, err)
	assert.Zero(t, report.Deleted, "no record was created")
}

func TestIntegration_TamperedObjectIsRejected(t *testing.T) {
	e := newEnv(t, true)
	user := e.user(t)
	blob, err := e.service.Write(t.Context(), user, bytes.NewReader([]byte("integrity matters")))
	require.NoError(t, err)
	files := e.objects(t)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(files[0], data, 0o600))

	_, err = e.service.Open(t.Context(), user, blob.ID)
	require.ErrorIs(t, err, envelope.ErrDecrypt)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("upload interrupted") }
