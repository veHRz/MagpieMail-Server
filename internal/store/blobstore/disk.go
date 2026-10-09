package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Disk stores objects as files under a root directory, sharded by the first
// characters of their key: root/ab/cd/abcd…. Files are readable by the server
// user only.
type Disk struct {
	root string
	tmp  string
}

var _ ObjectStore = (*Disk)(nil)

// NewDisk returns a store rooted at root, creating the directory if needed.
func NewDisk(root string) (*Disk, error) {
	tmp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, fmt.Errorf("blob store %q: %w", root, err)
	}
	return &Disk{root: root, tmp: tmp}, nil
}

func (d *Disk) path(key string) string {
	return filepath.Join(d.root, key[0:2], key[2:4], key)
}

// Put writes to a temporary file in the same file system, syncs it, then
// renames it into place and syncs the directory, so that a crash never leaves
// a partial object under the key.
func (d *Disk) Put(_ context.Context, key string, r io.Reader) (err error) {
	if err := checkKey(key); err != nil {
		return err
	}
	f, err := os.CreateTemp(d.tmp, "put-*")
	if err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()

	if _, err = io.Copy(f, r); err != nil {
		return fmt.Errorf("blobstore: writing %s: %w", key, err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}

	final := d.path(key)
	dir := filepath.Dir(final)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}
	if err = os.Rename(f.Name(), final); err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}
	return syncDir(dir)
}

// Get opens the object's file.
func (d *Disk) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	f, err := os.Open(d.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("blobstore: %w", err)
	}
	return f, nil
}

// Exists reports whether the object's file exists.
func (d *Disk) Exists(_ context.Context, key string) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	_, err := os.Stat(d.path(key))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("blobstore: %w", err)
	}
}

// Delete removes the object's file.
func (d *Disk) Delete(_ context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := os.Remove(d.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("blobstore: %w", err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("blobstore: %w", err)
	}
	return nil
}
