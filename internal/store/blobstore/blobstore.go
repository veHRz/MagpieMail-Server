// Package blobstore stores immutable objects (raw messages, attachments,
// documents) by key. Keys are lowercase hexadecimal fingerprints, chosen by
// package blobs; this package never sees who owns an object.
package blobstore

import (
	"context"
	"errors"
	"io"
	"regexp"
)

var (
	// ErrNotFound means no object has this key.
	ErrNotFound = errors.New("blobstore: object not found")
	// ErrInvalidKey means a key is not lowercase hexadecimal of 16 to 128
	// characters. Keys become file paths: nothing else may reach them.
	ErrInvalidKey = errors.New("blobstore: invalid key")
)

var validKey = regexp.MustCompile(`^[0-9a-f]{16,128}$`)

// ObjectStore is a backend for blob contents.
type ObjectStore interface {
	// Put stores the content read from r under key, atomically: readers see
	// either the previous object or the complete new one, never a part.
	Put(ctx context.Context, key string, r io.Reader) error
	// Get opens the object for reading.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Exists reports whether an object is stored under key.
	Exists(ctx context.Context, key string) (bool, error)
	// Delete removes the object. Deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}

func checkKey(key string) error {
	if !validKey.MatchString(key) {
		return ErrInvalidKey
	}
	return nil
}
