# 0022. Developer tooling and quality gates

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Local runs and CI must execute the same checks with the same tool versions.

## Decision

- Task (`Taskfile.yml`) is the entry point: `check`, `test:integration`, `audit`,
  `gen`, `smoke`, `dev`.
- golangci-lint v2 (with gofumpt and goimports) and gitleaks are installed into
  `.bin/` by `scripts/install-tools.sh`, at pinned versions, after checksum
  verification. golangci-lint is not built from source, as upstream advises.
- govulncheck and actionlint are pinned as Go tools in a separate `tools/go.mod`,
  so that their dependencies never enter the server's module graph.
- The architecture rules of CLAUDE.md are enforced by the linter: depguard keeps
  I/O out of `internal/domain` and bans backtracking regex engines; forbidigo
  bans outbound network APIs outside `internal/egress`.
- Unit tests run with `-race` whenever a C compiler is available (always in CI).
- Integration tests carry the `integration` build tag and use Testcontainers.

## Consequences

A contributor needs Go, Task, Docker and curl; `task tools` fetches the rest.
