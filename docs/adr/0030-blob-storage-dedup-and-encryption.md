# 0030. Blob storage: per-user deduplication, keyed fingerprints, encryption

- Status: Accepted
- Date: 2026-10-09
- Deciders: veHRz

## Context

ADR 0006 stores files as blobs addressed by a SHA-256 fingerprint, deduplicated,
optionally encrypted. Two questions remained:

- Should deduplication work across users? Global deduplication lets a user learn
  that someone else stores a given file (a confirmation-of-file attack, through
  timing or quotas), and it is impossible with blobs encrypted under per-user
  keys.
- Should the database store the plain SHA-256 of contents? Anyone reading the
  database could then confirm that a user holds a known file.

## Decision

- **Deduplication** is per user. The fingerprint is HMAC-SHA256 of the content
  under a fingerprint key derived from the user's root key (ADR 0031): stable for
  one user, unrelated across users, meaningless without the keys. `blobs` is
  unique on `(user_id, fingerprint)`; the object is named by the hexadecimal
  fingerprint.
- **Encryption.** Contents are sealed with the owner's data key (AES-256-GCM),
  bound to the owner and the object name. It is on by default
  (`storage.encrypt_blobs`). A whole content is sealed in memory, which is fine
  for mail-sized files; streaming encryption can come later.
- **Writing.** Content is spooled to a temporary file while fingerprinted (the
  stack mounts `/tmp` as tmpfs), recorded with an upsert, then stored. A
  repeated write touches the record and rewrites the object if it is missing.
- **Object store.** `blobstore.ObjectStore` is the interface, with a local disk
  backend:
  - files are sharded as `ab/cd/<key>` and readable by the server user only;
  - writes are atomic: temporary file, fsync, rename, then fsync of the directory;
  - keys are strict lowercase hexadecimal.

  Every backend must pass the `blobstoretest` conformance suite.
- **Reference counting.** Referencing rows count themselves (`AddRef`/`Release`)
  in their own transaction. The purge removes blobs without references after
  `storage.purge_grace` (1 hour), which protects a blob written but not yet
  referenced. For each blob, the purge deletes the object, then the record,
  inside the transaction that locks the record, so that a concurrent write of the
  same content waits and then stores a new object.
- **S3-compatible backend.** Deferred: it needs outbound network access through
  `internal/egress` (ADR 0013), which does not exist yet. It will be added
  behind the same interface and conformance suite.

## Consequences

Duplicates across users cost disk space; real duplicates are mostly within one
user (Gmail labels, the same invoice received twice). Backups must include the
master key to restore encrypted blobs.
