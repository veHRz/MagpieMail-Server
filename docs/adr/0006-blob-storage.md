# 0006. Content-addressed blob storage outside the database

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Raw messages (.eml), attachments and documents grow fast. Keeping them in
PostgreSQL makes the database heavy to back up and restore.

## Decision

Files are blobs addressed by their SHA-256 digest, behind an interface: local
disk by default, S3-compatible storage as an option. Writes are atomic, identical
content is stored once (reference counting), and orphans are purged. Blobs can
be encrypted with the same envelope mechanism as secrets (ADR 0014).

## Consequences

Backups must cover the database, the blobs and the master key together (phase S19).

## Alternatives considered

Everything in the database: simpler, but a bloated database.
