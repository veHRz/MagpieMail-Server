# 0003. REST API described by OpenAPI 3.1, contract first

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The client lives in another repository and must progress independently. Hooks,
scripts and SDKs (Python, Rust) also call the API.

## Decision

- The API is REST over HTTPS, described by `api/openapi.yaml` (OpenAPI 3.1),
  which is the single source of truth shared with the client.
- The contract is written and linted before the code of each domain. The server
  code is generated from it with oapi-codegen in strict mode, on top of chi.
- `/api/v1` only grows; a breaking change opens `/api/v2`. Errors use RFC 9457
  (`application/problem+json`), lists use cursor pagination, dates are UTC ISO 8601.
- The contract is versioned with semver; each published version is tagged `api-vX.Y.Z`.

## Consequences

The client develops against a mock server (Prism) generated from the contract.
CI fails when generated code is stale (from phase S1).

## Alternatives considered

gRPC: efficient, but harder to call from a hook, a shell script or a webhook.
