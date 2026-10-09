-- name: UpsertBlob :one
-- Inserts a blob, or touches the existing one with the same content so that
-- the purge grace period starts again.
INSERT INTO blobs (id, user_id, fingerprint, size, encrypted)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, fingerprint) DO UPDATE SET updated_at = now()
RETURNING *, (xmax = 0)::boolean AS inserted;

-- name: GetBlob :one
SELECT * FROM blobs WHERE id = $1 AND user_id = $2;

-- name: AddBlobRef :one
UPDATE blobs
SET ref_count = ref_count + 1, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING ref_count;

-- name: ReleaseBlobRef :one
UPDATE blobs
SET ref_count = ref_count - 1, updated_at = now()
WHERE id = $1 AND user_id = $2 AND ref_count > 0
RETURNING ref_count;

-- name: LockPurgeableBlobs :many
-- Unreferenced blobs past the grace period, locked until the end of the
-- transaction. Rows still pointed at (a missed reference count) are skipped.
SELECT b.* FROM blobs b
WHERE b.ref_count = 0
  AND b.updated_at < sqlc.arg(older_than)
  AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.raw_blob_id = b.id)
  AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.blob_id = b.id)
  AND NOT EXISTS (SELECT 1 FROM outbox o WHERE o.blob_id = b.id)
ORDER BY b.updated_at
LIMIT sqlc.arg(max_blobs)
FOR UPDATE OF b SKIP LOCKED;

-- name: DeleteBlob :exec
DELETE FROM blobs WHERE id = $1;
