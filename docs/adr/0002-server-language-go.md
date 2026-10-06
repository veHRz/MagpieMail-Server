# 0002. Write the server in Go

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The server holds all the business logic: hundreds of concurrent IMAP
connections, MIME parsing, PGP, user-supplied regular expressions, an HTTP and
WebSocket API. Changing language after phase S5 would mean a rewrite.

## Decision

The server is written in Go, latest stable release (1.27 at the start of the
project), as a single binary `magpie`.

## Consequences

- Mature mail libraries: emersion/go-imap v2, go-smtp, go-message.
- PGP maintained by Proton (gopenpgp v3).
- `regexp` is RE2: linear-time matching of user regexes comes for free (ADR 0015).
- Goroutines make many simultaneous IMAP connections cheap.
- A static binary gives a small distroless image (ADR 0021) and fast TDD cycles.

## Alternatives considered

- Rust (Axum): strong safety, but slower iteration and a thinner mail ecosystem.
- Python (FastAPI): fast to write, but weaker for many long-lived connections and
  CPU-bound parsing.
