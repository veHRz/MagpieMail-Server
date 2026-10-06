# 0019. Structured logs and request IDs

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Logs must be machine-readable and correlate the records of one request, without
exposing personal data: "never log message content, tokens or passwords".

## Decision

- `log/slog`, JSON by default (`log.format: text` for humans), timestamps in UTC.
- Every request gets an ID: a well-formed incoming `X-Request-ID`
  (`[A-Za-z0-9._-]{1,64}`, as set by a reverse proxy) is reused, anything else is
  replaced by a fresh UUIDv7. The ID is echoed in the response and stored in the
  context; the logger adds it as `request_id` to every record logged with that
  context.
- The access log records method, route pattern, status, size and duration. It
  never records the raw path or query string, the client address or the user
  agent. Successful health probes are logged at debug level.
- Linting (sloglint) enforces snake_case keys, constant messages and the
  `…Context` methods whenever a context is in scope.

## Consequences

Phase S1 extends the middleware chain (panic recovery, limits, security headers)
on top of this base.
