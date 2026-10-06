# 0013. A single egress module for all outbound traffic

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The server must be self-sufficient: it talks only to mail providers, unless the
admin enables other features (external AI, Web Push, PGP key discovery, remote
image proxy, webhooks, unsubscribe links).

## Decision

All outbound network traffic goes through `internal/egress`. It logs every call,
can be disabled per feature, and can route through an upstream proxy or VPN
(Gluetun). golangci-lint's forbidigo rules forbid outbound network APIs
(`http.Client`, `http.Get`, `net.Dial`…) everywhere else.

The only exception is `magpie healthcheck`, which probes the server's own
`/readyz` over loopback; it carries an explained `nolint` directive.

## Consequences

"Which features talk to the Internet?" has one answer, enforced by the linter and
testable (phases S10, S14: "no call to X when disabled").
