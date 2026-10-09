package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

// Blob is the record of a stored file. Its content lives in the blob store,
// named by its fingerprint; package blobs reads and writes both together.
type Blob = gen.Blob

// Blobs is the repository of blob records.
type Blobs struct{ s *Store }

// Blobs returns the blob repository.
func (s *Store) Blobs() Blobs { return Blobs{s} }

// Upsert records a blob, or returns the user's existing blob with the same
// fingerprint, touched so that its purge grace period starts again.
func (r Blobs) Upsert(ctx context.Context, userID uuid.UUID, fingerprint []byte, size int64, encrypted bool) (Blob, bool, error) {
	row, err := r.s.q.UpsertBlob(ctx, gen.UpsertBlobParams{
		ID: newID(), UserID: userID, Fingerprint: fingerprint, Size: size, Encrypted: encrypted,
	})
	if err != nil {
		return Blob{}, false, translate(err)
	}
	return Blob{
		ID: row.ID, UserID: row.UserID, Fingerprint: row.Fingerprint, Size: row.Size,
		Encrypted: row.Encrypted, RefCount: row.RefCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, row.Inserted, nil
}

// Get returns one of the user's blobs.
func (r Blobs) Get(ctx context.Context, userID, id uuid.UUID) (Blob, error) {
	b, err := r.s.q.GetBlob(ctx, gen.GetBlobParams{ID: id, UserID: userID})
	return b, translate(err)
}

// AddRef counts one more reference to the blob and returns the new count.
// Call it in the transaction that writes the referencing row.
func (r Blobs) AddRef(ctx context.Context, userID, id uuid.UUID) (int32, error) {
	n, err := r.s.q.AddBlobRef(ctx, gen.AddBlobRefParams{ID: id, UserID: userID})
	return n, translate(err)
}

// Release counts one reference less and returns the new count. A blob without
// references is purged once the grace period is over.
func (r Blobs) Release(ctx context.Context, userID, id uuid.UUID) (int32, error) {
	n, err := r.s.q.ReleaseBlobRef(ctx, gen.ReleaseBlobRefParams{ID: id, UserID: userID})
	return n, translate(err)
}

// LockPurgeable returns up to limit blobs without references, last touched
// before olderThan, locked until the end of the transaction.
func (r Blobs) LockPurgeable(ctx context.Context, olderThan time.Time, limit int32) ([]Blob, error) {
	blobs, err := r.s.q.LockPurgeableBlobs(ctx, gen.LockPurgeableBlobsParams{OlderThan: olderThan, MaxBlobs: limit})
	return blobs, translate(err)
}

// Delete removes a blob record.
func (r Blobs) Delete(ctx context.Context, id uuid.UUID) error {
	return translate(r.s.q.DeleteBlob(ctx, id))
}
