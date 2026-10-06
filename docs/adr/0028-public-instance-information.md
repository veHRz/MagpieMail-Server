# 0028. Public instance information without the server build

- Status: Accepted
- Date: 2026-10-07
- Deciders: veHRz

## Context

`GET /api/v1/info` must tell a client, before sign-in, whether it can talk to
this server and which optional features exist. Publishing the exact server build
would help an attacker match the instance to known vulnerabilities.

## Decision

The operation is public and returns only:

- `apiVersion`, the contract version;
- `features`: whether AI, Document Mode and Web Push are enabled. All are
  `false` in S1, and new features are added as new members.

The server version is not part of it; administrators will see it through the
admin API (phase S4).

## Consequences

Clients check compatibility with `apiVersion`. The container image label and
`magpie --version` still give operators the build.
