// Package blobs stores users' files: blob records in PostgreSQL, contents in
// an object store, named by the owner's keyed fingerprint of the content.
//
// Equal content of one user is stored once; users never share a blob (ADR
// 0030). Contents are encrypted with the owner's data key unless encryption
// is turned off. A blob without references is purged after a grace period.
package blobs

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/blobstore"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
)

const purgeBatch = 100

// Options configures a Service.
type Options struct {
	// Encrypt seals contents with the owner's data key.
	Encrypt bool
	// Grace is how long an unreferenced blob is kept.
	Grace time.Duration
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
}

// Service reads, writes and purges blobs.
type Service struct {
	store   *postgres.Store
	objects blobstore.ObjectStore
	opts    Options
}

// NewService returns a blob service. The store must hold the master keyring:
// fingerprints are keyed per user even when contents are not encrypted.
func NewService(store *postgres.Store, objects blobstore.ObjectStore, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{store: store, objects: objects, opts: opts}
}

// Write stores content for the user and returns its blob record, which is
// the existing one when the user already stored the same content. Reference
// the blob with postgres.Blobs.AddRef in the transaction that writes the
// referencing row; until then it counts as an orphan.
func (s *Service) Write(ctx context.Context, userID uuid.UUID, content io.Reader) (postgres.Blob, error) {
	key, err := s.store.Keys().ForUser(ctx, userID)
	if err != nil {
		return postgres.Blob{}, err
	}

	// The fingerprint is known only once the whole content is read: spool it.
	spool, err := os.CreateTemp("", "magpie-blob-*")
	if err != nil {
		return postgres.Blob{}, fmt.Errorf("blobs: %w", err)
	}
	defer func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}()
	fingerprint := key.NewFingerprint()
	size, err := io.Copy(io.MultiWriter(spool, fingerprint), content)
	if err != nil {
		return postgres.Blob{}, fmt.Errorf("blobs: reading content: %w", err)
	}

	blob, inserted, err := s.store.Blobs().Upsert(ctx, userID, fingerprint.Sum(nil), size, s.opts.Encrypt)
	if err != nil {
		return postgres.Blob{}, err
	}
	objectKey := hex.EncodeToString(blob.Fingerprint)
	if !inserted {
		// The object may be missing after a crash or a purge that failed half
		// way: write it again rather than trust the record.
		exists, err := s.objects.Exists(ctx, objectKey)
		if err != nil || exists {
			return blob, err
		}
	}

	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return postgres.Blob{}, fmt.Errorf("blobs: %w", err)
	}
	var stored io.Reader = spool
	if blob.Encrypted {
		plaintext, err := io.ReadAll(spool)
		if err != nil {
			return postgres.Blob{}, fmt.Errorf("blobs: %w", err)
		}
		sealed, err := key.Seal(plaintext, contentAAD(userID, objectKey))
		if err != nil {
			return postgres.Blob{}, err
		}
		stored = bytes.NewReader(sealed)
	}
	if err := s.objects.Put(ctx, objectKey, stored); err != nil {
		return postgres.Blob{}, err
	}
	return blob, nil
}

// Open returns the content of one of the user's blobs.
func (s *Service) Open(ctx context.Context, userID, blobID uuid.UUID) (io.ReadCloser, error) {
	blob, err := s.store.Blobs().Get(ctx, userID, blobID)
	if err != nil {
		return nil, err
	}
	objectKey := hex.EncodeToString(blob.Fingerprint)
	r, err := s.objects.Get(ctx, objectKey)
	if err != nil || !blob.Encrypted {
		return r, err
	}
	defer r.Close()

	sealed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("blobs: reading %s: %w", blobID, err)
	}
	key, err := s.store.Keys().ForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	plaintext, err := key.Open(sealed, contentAAD(userID, objectKey))
	if err != nil {
		return nil, fmt.Errorf("blobs: opening %s: %w", blobID, err)
	}
	return io.NopCloser(bytes.NewReader(plaintext)), nil
}

// PurgeReport tells what a purge removed.
type PurgeReport struct {
	Deleted int
	Bytes   int64
}

// Purge removes blobs without references whose grace period is over: the
// object, then the record, in the transaction that locks the record. A write
// of the same content waits for that transaction, then stores a new object.
func (s *Service) Purge(ctx context.Context) (PurgeReport, error) {
	var report PurgeReport
	olderThan := s.opts.Now().Add(-s.opts.Grace)
	for {
		batch := 0
		err := s.store.InTx(ctx, func(tx *postgres.Store) error {
			purgeable, err := tx.Blobs().LockPurgeable(ctx, olderThan, purgeBatch)
			if err != nil {
				return err
			}
			batch = len(purgeable)
			for _, blob := range purgeable {
				if err := s.objects.Delete(ctx, hex.EncodeToString(blob.Fingerprint)); err != nil {
					return err
				}
				if err := tx.Blobs().Delete(ctx, blob.ID); err != nil {
					return err
				}
				report.Deleted++
				report.Bytes += blob.Size
			}
			return nil
		})
		if err != nil {
			return report, errors.Join(errors.New("blobs: purge interrupted"), err)
		}
		if batch < purgeBatch {
			return report, nil
		}
	}
}

// contentAAD binds a sealed content to its owner and name.
func contentAAD(userID uuid.UUID, objectKey string) []byte {
	return envelope.AAD("blobs", userID.String(), objectKey)
}
