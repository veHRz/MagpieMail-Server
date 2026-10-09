package blobstore_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/store/blobstore"
	"github.com/veHRz/MagpieMail-Server/internal/store/blobstore/blobstoretest"
)

func newDisk(t *testing.T) (*blobstore.Disk, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "blobs")
	d, err := blobstore.NewDisk(root)
	require.NoError(t, err)
	return d, root
}

func TestDisk_Conformance(t *testing.T) {
	blobstoretest.Run(t, func(t *testing.T) blobstore.ObjectStore {
		d, _ := newDisk(t)
		return d
	})
}

func TestDisk_ShardsFilesByKeyPrefix(t *testing.T) {
	d, root := newDisk(t)
	key := blobstoretest.NewKey()

	require.NoError(t, d.Put(t.Context(), key, strings.NewReader("x")))

	info, err := os.Stat(filepath.Join(root, key[0:2], key[2:4], key))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm(), "only the server reads blobs")
	dir, err := os.Stat(filepath.Join(root, key[0:2]))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), dir.Mode().Perm())
}

func TestDisk_LeavesNoTemporaryFiles(t *testing.T) {
	d, root := newDisk(t)
	key := blobstoretest.NewKey()
	require.NoError(t, d.Put(t.Context(), key, strings.NewReader("ok")))
	require.Error(t, d.Put(t.Context(), blobstoretest.NewKey(), iotestErrReader{}))

	entries, err := os.ReadDir(filepath.Join(root, "tmp"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestNewDisk_RefusesAnUnusableDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := blobstore.NewDisk(file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), file)
}

type iotestErrReader struct{}

func (iotestErrReader) Read([]byte) (int, error) { return 0, os.ErrClosed }
