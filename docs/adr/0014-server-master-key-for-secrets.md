# 0014. Encrypt secrets with a server master key

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Accounts are synchronized in the background, without the user. The server must
therefore decrypt account credentials (OAuth refresh tokens, passwords) on its own.

## Decision

Secrets are encrypted at rest with envelope encryption: a master key supplied by
an environment variable or a Docker secret, and one data key per user.
`magpie admin rotate-key` re-encrypts everything (phase S2).

## Consequences

Losing the master key loses every stored credential: backups and documentation
must insist on saving it outside the server. Secrets never appear in logs; the
configuration's `Secret` type redacts them in every output format.

## Alternatives considered

Encrypting with the user's password: background sync would be impossible while
the user is logged out.
