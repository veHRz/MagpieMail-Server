# 0032. Migrations with goose, typed queries with sqlc, both from the binary or a container

- Status: Accepted
- Date: 2026-10-09
- Deciders: veHRz

## Context

ADR 0004 chose goose and sqlc. sqlc's PostgreSQL parser needs cgo, which the
development machines may lack, and the deployment must migrate before serving.

## Decision

- Migrations are SQL files in `migrations/`, with `-- +goose Up` and `Down`
  sections, embedded in the binary.
  - `magpie migrate up|down|status` applies them.
  - A test applies every migration up, rolls every one back, then applies them
    again, on an empty PostgreSQL 18.
- `magpie serve` does not migrate by itself (phase S19 decides on optional
  migrations at startup). The compose stack runs a one-shot `migrate` service,
  and the server starts once it has succeeded.
- sqlc runs from its pinned container image (`scripts/sqlc.sh`), against the
  migrations as schema, into `internal/store/postgres/gen`.
  - The generated code is committed.
  - `task gen` regenerates it, and `task gen:check` (part of `task check`) runs
    `sqlc diff` to fail on stale code.
- Integration tests share one PostgreSQL container per test binary. Each test
  gets its own database, copied from a migrated template, which takes about
  60 ms instead of seconds.

## Consequences

Contributors need Docker for code generation, as for the contract linter. A
schema change is a new migration plus `task gen`.
