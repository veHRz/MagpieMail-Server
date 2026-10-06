# 0023. Contract tooling: Redocly for linting and publishing, oapi-codegen for the server

- Status: Accepted
- Date: 2026-10-07
- Deciders: veHRz

## Context

ADR 0003 makes `api/openapi.yaml` (OpenAPI 3.1) the source of truth. Phase S1
requires linting it with house rules (operationId, tag, examples and error
responses on every operation), generating the server from it, and failing CI
when the generated code is stale. The plan allowed Redocly or Spectral.

## Decision

- **Redocly CLI** lints, bundles and renders the contract. It runs from its
  pinned container image (`scripts/redocly.sh`, as the calling user), so no
  Node.js is needed on development machines; `task check` therefore needs Docker,
  like the integration tests.
- `redocly.yaml` extends `recommended-strict`, where every finding is an error,
  so "no warning" is enforced. On top come configurable house rules:
  - every operation has a tag, its 500 response, and an example on every body;
  - operations under `/api` state their security explicitly and declare 429;
  - secured operations declare 401;
  - operations with parameters declare 400;
  - operations with a body declare 400 and 413;
  - operations on a path with parameters declare 404.

  The generic `operation-4xx-response` rule is replaced by these.
- `scripts/lint-contract.sh` also lints `test/contract/rules-fixture.yaml`, which
  breaks each house rule once, and fails if any rule stays silent.
- Deliberate exceptions live in `.redocly.lint-ignore.yaml`, each explained. In
  S1 they cover shared components (pagination, event envelope, error responses)
  that the plan requires before any operation uses them.
- **oapi-codegen** v2.8.0 (first release supporting OpenAPI 3.1) generates models,
  the strict server interface, chi routes and the embedded contract into
  `internal/transport/httpapi/oapi`. The generated code is committed; `task gen`
  regenerates it and `task gen:check`, part of `task lint`, fails when it differs.
- The server validates requests against the embedded contract at run time with
  kin-openapi (ADR 0024), and tests validate every response against it
  (`internal/testsupport/contracttest`).

## Consequences

The contract cannot drift from the code in either direction: requests and
responses are checked against it, and the route test proves that the served
routes and the contract's operations are the same set.

## Alternatives considered

Spectral: comparable rules, but no HTML rendering. vacuum: a Go-native,
Spectral-compatible linter that avoids Docker, but it generates no reference
documentation, so Redocly would still be needed.
