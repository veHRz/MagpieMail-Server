# 0018. Package names that do not shadow the standard library

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The plan's target layout names two packages after standard library packages
that the same code imports constantly: `internal/sync` (vs `sync`) and
`internal/transport/http` (vs `net/http`). Every importer would need an alias.

## Decision

- `internal/sync` is named `internal/mailsync`.
- `internal/transport/http` is named `internal/transport/httpapi`.
- Packages not listed in the plan's layout are added where S0 needs them:
  `internal/config`, `internal/observability` (logging, request IDs),
  `internal/cli`, `internal/buildinfo`, and `internal/testsupport` (integration
  test helpers).
- Every planned package exists from S0 with a `doc.go` stating its
  responsibility, so the layout is visible in the code.

## Consequences

The plan's tree in `docs/plan/01-server-plan.md` differs on these two names;
this ADR is the reference. Sub-packages of `internal/mail` (IMAP, SMTP, MIME)
will avoid stdlib names too when they are created in phase S5.
