# 0011. Continuous integration with GitHub Actions

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The repository is hosted on GitHub; a self-hosted Forgejo or Gitea remains a
possible home.

## Decision

CI runs on GitHub Actions (`.github/workflows/ci.yml`), written to stay
compatible with Forgejo and Gitea Actions:

- only first-party actions (checkout, setup-go), pinned by commit SHA;
- every other tool is installed by the repository's scripts, at pinned versions
  with checksum verification;
- each job runs a `task` target, so CI and local runs execute the same commands.

Jobs: `task check` (format, lint, unit tests with the race detector),
`task test:integration`, `task audit` (govulncheck, gitleaks over the whole
history) and `task smoke` (image and compose stack).

Renovate keeps Go modules, images (by digest), actions and pinned tools current.

## Consequences

Formatting, lint, tests, vulnerability and secret scans are blocking.

## Alternatives considered

GitLab CI: not where the code lives. Marketplace actions for golangci-lint or
gitleaks: GitHub-specific, and the gitleaks action needs a licence for organizations.
