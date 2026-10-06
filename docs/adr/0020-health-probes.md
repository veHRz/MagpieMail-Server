# 0020. Health probes outside the versioned API

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

S0 needs `/healthz` and `/readyz` before the API contract exists (S1), while the
contract-first rule and the S1 test "every served route exists in the contract"
would otherwise conflict with them.

## Decision

- `/healthz` (liveness) answers 200 as long as the process serves HTTP; it checks
  no dependency, so an orchestrator never restarts the server because PostgreSQL
  is down.
- `/readyz` (readiness) pings PostgreSQL with a 2-second timeout: 200
  `{"status":"ok"}`, or 503 `application/problem+json` without internal details
  (they go to the logs).
- Both live outside `/api/v1`, need no authentication and are not cached.
- In phase S1 they are added to `api/openapi.yaml` under a `health` tag, so that
  every served route is in the contract.
- The server starts without waiting for the database; readiness reports it.

## Consequences

The overview's tag list gains `health` in S1.
