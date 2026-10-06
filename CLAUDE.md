# CLAUDE.md — magpiemail-server

## Project
MagpieMail server: a self-hosted, privacy-first mail aggregation server
(multi-account, multi-user). All business logic lives here. Clients live in a
separate repo and only know this server through `api/openapi.yaml`.

## Language rules
- Always talk to the user in French.
- Everything committed is in English: code, identifiers, comments, commit
  messages, PR descriptions, docs, ADRs, log and error messages.
- Exception: `docs/plan/` is the user's plan, written in French.

## How to work
- Read `docs/plan/` first. Work on ONE phase at a time, as asked (e.g. "S5").
- Check in `docs/PROGRESS.md` that the phase's dependencies are done.
  If not, stop and tell the user.
- Contract first: update and lint `api/openapi.yaml` before implementing.
- TDD: failing test first, then minimal code, then refactor.
- Never tick a validation criterion without proof (test name, command output).
- Record non-trivial decisions as ADRs in `docs/adr/`.
- End of phase: run `task check`, update `docs/PROGRESS.md` and
  `CHANGELOG.md`, summarize in French, then stop for review.
- If the plan is ambiguous or looks wrong, ask instead of guessing.

## Commands
- `task check`            format, lint, unit tests
- `task test:integration` integration tests (Docker required)
- `task gen`              regenerate code from OpenAPI and SQL
- `task dev`              run the full stack with docker compose
- `task audit`            govulncheck and gitleaks
- `task smoke`            build the image, check size, user and readiness time

## Architecture rules
- `internal/domain` has no I/O dependencies.
- All outbound network traffic goes through `internal/egress`.
- Never log message content, tokens or passwords.
- User-supplied regexes: Go `regexp` (RE2) only, with size limits.
- Secrets are encrypted at rest with the server master key.
- Package names never shadow the standard library (`internal/mailsync`,
  `internal/transport/httpapi`; see ADR 0018).

## Conventions
Conventional Commits; branches `phase/SNN-short-name`; gofumpt and
golangci-lint clean; UUIDv7 ids; UTC timestamps; RFC 9457 errors.
