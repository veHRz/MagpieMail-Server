# 0026. Security schemes, secure by default, and the authentication hook

- Status: Accepted
- Date: 2026-10-07
- Deciders: veHRz

## Context

The contract needs its security schemes in S1, but authentication itself
arrives in S3. A new operation must never be public by accident.

## Decision

- Two schemes:
  - `bearerAuth`, for native clients, SDKs, hooks and personal API tokens. The
    token is opaque (`bearerFormat: opaque`), not a JWT the client could
    inspect.
  - `cookieAuth`, the web client's session cookie `__Host-magpie_session`. The
    `__Host-` prefix forces `Secure`, `Path=/` and no `Domain`. The cookie is
    `HttpOnly` and `SameSite=Strict`, and S3 adds CSRF protection.
- The contract's global `security` requires either scheme: operations are
  protected unless they declare `security: []`. A house rule requires operations
  under `/api` to state their security explicitly.
- The `Authenticator` interface finds the request's principal and stores it in
  the context; it rejects nothing itself. The contract validator then enforces
  each operation's requirements: a requirement is met only by a principal
  authenticated through that scheme. Failures get 401 with
  `WWW-Authenticate: Bearer realm="magpiemail"`.
- Until S3, no authenticator is installed: every protected operation answers 401.

## Consequences

S3 implements `Authenticator` and its storage; the chain, the 401s and the
per-user rate limit already work, as tests with a fake authenticator show.
