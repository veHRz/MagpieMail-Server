// Package blobstoretest is the conformance suite of blobstore.ObjectStore:
// every backend (local disk now, S3-compatible later) must pass it.
package blobstoretest

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/store/blobstore"
)

// NewKey returns a random valid key.
func NewKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func read(t *testing.T, s blobstore.ObjectStore, key string) []byte {
	t.Helper()
	r, err := s.Get(t.Context(), key)
	require.NoError(t, err)
	defer r.Close()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return data
}

type failingReader struct{ sent bool }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.sent {
		return 0, errors.New("connection lost")
	}
	f.sent = true
	return copy(p, "partial content"), nil
}

// Run checks the behaviour every ObjectStore must have. newStore returns an
// empty store.
func Run(t *testing.T, newStore func(t *testing.T) blobstore.ObjectStore) {
	t.Run("put then get", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		content := bytes.Repeat([]byte("magpie "), 10_000)

		require.NoError(t, s.Put(t.Context(), key, bytes.NewReader(content)))

		assert.Equal(t, content, read(t, s, key))
		exists, err := s.Exists(t.Context(), key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("put again replaces", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		require.NoError(t, s.Put(t.Context(), key, strings.NewReader("first")))
		require.NoError(t, s.Put(t.Context(), key, strings.NewReader("second")))
		assert.Equal(t, "second", string(read(t, s, key)))
	})

	t.Run("missing object", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		_, err := s.Get(t.Context(), key)
		require.ErrorIs(t, err, blobstore.ErrNotFound)
		exists, err := s.Exists(t.Context(), key)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("delete is idempotent", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		require.NoError(t, s.Put(t.Context(), key, strings.NewReader("x")))
		require.NoError(t, s.Delete(t.Context(), key))
		require.NoError(t, s.Delete(t.Context(), key))
		_, err := s.Get(t.Context(), key)
		require.ErrorIs(t, err, blobstore.ErrNotFound)
	})

	t.Run("a failed put leaves nothing", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		require.Error(t, s.Put(t.Context(), key, &failingReader{}))
		exists, err := s.Exists(t.Context(), key)
		require.NoError(t, err)
		assert.False(t, exists, "no partial object is ever visible")
	})

	t.Run("a failed put keeps the previous object", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		require.NoError(t, s.Put(t.Context(), key, strings.NewReader("complete")))
		require.Error(t, s.Put(t.Context(), key, &failingReader{}))
		assert.Equal(t, "complete", string(read(t, s, key)))
	})

	t.Run("invalid keys are refused", func(t *testing.T) {
		s := newStore(t)
		for _, key := range []string{"", "../../etc/passwd", "ab/cd", "ABCDEF0123456789", "xyz0123456789abc", "0123"} {
			require.ErrorIs(t, s.Put(t.Context(), key, strings.NewReader("x")), blobstore.ErrInvalidKey, key)
			_, err := s.Get(t.Context(), key)
			require.ErrorIs(t, err, blobstore.ErrInvalidKey, key)
			_, err = s.Exists(t.Context(), key)
			require.ErrorIs(t, err, blobstore.ErrInvalidKey, key)
			require.ErrorIs(t, s.Delete(t.Context(), key), blobstore.ErrInvalidKey, key)
		}
	})

	t.Run("concurrent puts of the same content", func(t *testing.T) {
		s, key := newStore(t), NewKey()
		content := bytes.Repeat([]byte("same "), 50_000)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				assert.NoError(t, s.Put(t.Context(), key, bytes.NewReader(content)))
			})
		}
		wg.Wait()
		assert.Equal(t, content, read(t, s, key), "never a mix of two writes")
	})
}
