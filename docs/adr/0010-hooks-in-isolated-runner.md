# 0010. Run user hooks in an isolated hook-runner container

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

A user script run by the server is arbitrary code execution.

## Decision

Hooks run in a separate `hook-runner` container: no secrets, no database access,
network off by default, CPU, memory and time limits, read-only file system except
a temporary directory, non-root user. A hook receives its event as JSON on stdin
and a short-lived, scoped API token. Python and shell are supported first.

## Consequences

The server never executes user code itself. Hooks reach MagpieMail only through
the public API, within their token's scopes.

## Alternatives considered

WebAssembly (Extism): stronger isolation, but harder for users to write. Possible later.
