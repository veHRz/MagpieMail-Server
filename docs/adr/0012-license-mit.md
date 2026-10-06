# 0012. MIT licence

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The overview left the licence open between AGPL-3.0 (hosted forks must stay
open) and a permissive licence.

## Decision

The server, its API contract and its SDKs are released under the MIT licence
(`LICENSE`).

## Consequences

Anyone may use, modify and host MagpieMail, including in closed forks. The
client can embed the contract and the generated code without licensing doubts,
and hooks importing the SDKs are unaffected. All dependencies must be compatible
with MIT distribution; licences are reviewed when adding a dependency.

## Alternatives considered

AGPL-3.0: keeps hosted forks open, but would have required a separate licence for
the contract and the SDKs. Apache-2.0: adds an explicit patent grant, at the cost
of more notice obligations.
