# 0024. One middleware chain, validated against the contract, for every request

- Status: Accepted
- Date: 2026-10-07
- Deciders: veHRz

## Context

Every domain added from S3 on must inherit the same error format, limits and
security without re-implementing them.

## Decision

Every request goes through the same chain (`newChain`), in this order:

1. Request ID, then the access log (ADR 0019).
2. Panic recovery: logged with its stack; the client gets a 500 problem.
3. Security headers on every response:
   - `X-Content-Type-Options: nosniff`;
   - `Referrer-Policy: no-referrer`;
   - `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`;
   - `X-Frame-Options: DENY`;
   - `Cache-Control: no-store`;
   - `Strict-Transport-Security`, only when `server.hsts_max_age` is set
     (usually the TLS proxy sets it).
4. CORS, only for the origins listed in `server.cors.allowed_origins`, with
   credentials. There is no wildcard, and CORS is off by default: the web client
   should be served from the API's origin. CORS comes before rate limiting so
   that browsers can read a 429 and its `Retry-After`.
5. Rate limiting per client address (ADR 0025); probes are exempt.
6. Body size limit (`server.max_body_bytes`, 1 MiB): 413 at once when the
   declared length is larger, or when reading passes the limit.
7. Authentication hook (ADR 0026), then rate limiting per user.
8. Validation against the contract with kin-openapi:
   - an unknown path gets 404;
   - an unknown method gets 405, with `Allow`;
   - an unmet security requirement gets 401, with `WWW-Authenticate`;
   - parameters, body and content type that do not match get 400.

   Handlers can trust the shape of their input.

Errors are RFC 9457 problems (`application/problem+json`):

- generic HTTP errors use `type: about:blank` and the status text as `title`;
- every problem carries the `requestId` member;
- `detail` never contains internal error messages: those go to the logs.

Problem types specific to MagpieMail will be defined when a domain needs one.

## Consequences

From S3 on, a new operation only needs its contract entry and its handler. The
contract must declare every error an operation can return; the tests check it.
