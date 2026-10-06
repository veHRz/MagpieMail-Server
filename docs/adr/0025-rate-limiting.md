# 0025. In-memory token-bucket rate limiting per address and per user

- Status: Accepted
- Date: 2026-10-07
- Deciders: veHRz

## Context

S1 requires rate limiting per IP and per user, with 429 and `Retry-After`. The
server usually sits behind a TLS reverse proxy, so the TCP peer is the proxy.

## Decision

- Token buckets (`golang.org/x/time/rate`), in memory:
  - one per client address, before authentication (defaults: 50 requests per
    second, bursts of 200);
  - one per authenticated user, across all their addresses (25/s, bursts of 100).

  Both are configurable and can be disabled (`server.rate_limit`).
- A limited request gets a 429 problem with `Retry-After` in whole seconds
  (rounded up, at least 1).
- `/healthz` and `/readyz` are never limited.
- The client address is the TCP peer, unless the peer is in
  `server.trusted_proxies`. Only then is `X-Forwarded-For` read, from the right,
  skipping trusted proxies: the first untrusted hop is the client. Hops further
  left are client-controlled and ignored. The list is empty by default, so a
  direct client cannot choose its address.
- Idle buckets are forgotten after 10 minutes, or after the time a bucket takes
  to refill if longer, so forgetting one never grants extra tokens.

## Consequences

Buckets are per process. Running several `magpie serve` instances multiplies
the effective limits; a shared store (PostgreSQL or a dedicated limiter) will
be needed then. Login brute-force protection is a separate mechanism (phase S3).
