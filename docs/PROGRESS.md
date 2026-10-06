# Progress

Phases come from [docs/plan/01-server-plan.md](plan/01-server-plan.md). A phase
is done only when every validation criterion is proven. Skipping a phase must be
recorded here.

| Phase | Name | Depends on | Status | Branch |
| --- | --- | --- | --- | --- |
| S0 | Repository foundations | — | In review (CI proof pending) | `phase/S00-foundations` |
| S1 | API contract and HTTP skeleton | S0 | Not started | |
| S2 | Data model and storage | S0 | Not started | |
| S3 | Authentication and multi-user | S1, S2 | Not started | |
| S4 | Administration and policies | S3 | Not started | |
| S5 | Accounts and providers | S4 | Not started | |
| S6 | Synchronization engine | S5 | Not started | |
| S7 | Reading, safe rendering, anti-tracking | S6 | Not started | |
| S8 | Global folders, tags, actions | S7 | Not started | |
| S9 | Composition, drafts and sending | S7 | Not started | |
| S10 | Real time and notifications | S6, S8 | Not started | |
| S11 | Search | S7 | Not started | |
| S12 | Rules engine | S8, S10 | Not started | |
| S13 | Hooks, webhooks and SDKs | S12 | Not started | |
| S14 | AI layer | S12, S13 | Not started | |
| S15 | PGP and antivirus | S9 | Not started | |
| S16 | Document Mode | S11 (S14 for AI) | Not started | |
| S17 | Calendar and contacts | S9 | Not started | |
| S18 | Import and export | S9, S17 | Not started | |
| S19 | Hardening, documentation, release | all retained phases | Not started | |

## S0 — Repository foundations

Goal: an empty but complete repository, where `task check` and CI pass and
`docker compose up` starts a server that answers.

### Validation criteria

- [ ] **`task check` passes locally and in CI.**
  - Local: proven on 2026-10-06. `task check` exits 0. It runs:
    - `golangci-lint fmt --diff` and `go mod tidy -diff` (no diff);
    - `golangci-lint run` (`0 issues.`);
    - `actionlint`;
    - `go test ./...`, with every package `ok`.
  - The unit tests also pass with `-race`: `go test -race -count=2 ./...` in
    `golang:1.27.1-trixie`. The development machine has no C compiler; CI has one.
  - CI: pending. The workflow `.github/workflows/ci.yml` passes actionlint, but
    has not run yet. This box gets ticked once the first GitHub Actions run of
    this branch is green.
- [x] **After `docker compose up`, `/readyz` answers 200 in less than 10 seconds.**
  - `task smoke` (`scripts/smoke.sh`) starts the compose stack on an empty
    database volume (so PostgreSQL's first-time initialization is included), with
    images already built and pulled.
  - It polls `/readyz` until the first 200:
    `PASS  /readyz answered 200 5461 ms after docker compose up (fresh volume)`.
  - Five runs: 5384, 5348, 5431, 5405 and 5461 ms.
  - Readiness tracks the database: `TestIntegration_ReadinessFollowsTheDatabase`
    runs `magpie serve` against PostgreSQL 18 (Testcontainers) and gets 200, then
    503 once the database is stopped.
- [x] **An invalid configuration fails startup with a message naming the faulty key (test).**
  - `internal/config`:
    - `TestLoad_InvalidConfigurationNamesTheKey`, 12 cases: unknown file key,
      unknown `MAGPIE_*` variable, section given a scalar, malformed duration,
      duration without unit, non-positive duration, invalid listen address, port
      out of range, unknown log level and format, missing database URL, wrong URL
      scheme;
    - `TestLoad_ReportsEveryProblemAtOnce`;
    - `ExampleLoad_invalidConfiguration`, which pins the exact message.
  - `internal/cli`: `TestServe_InvalidConfigurationFailsNamingTheKey` checks exit
    code 1 and the key in stderr when the bad value comes from an environment
    variable, from `--config` or from `MAGPIE_CONFIG`.
  - Image: `scripts/smoke.sh` runs the image with `MAGPIE_SERVER__LISTEN=nope`:
    `PASS  invalid configuration exits with code 1: magpie: invalid configuration: - server.listen (from MAGPIE_SERVER__LISTEN): must be a host:port address such as ":8080" or "127.0.0.1:8080", got "nope"`.
- [x] **The image runs without root and weighs less than 50 MB.** From `task smoke`:
  - `PASS  image user is 65532:65532 (non-root)`;
  - `PASS  server process runs as uid 65532`, read with `docker top` on the running
    container;
  - `PASS  image weighs 18 MB (18018783 bytes < 50000000)`.

### Also verified

- `task test:integration`: 42 top-level tests pass, including the three
  integration tests against PostgreSQL 18.
- `task audit`:
  - govulncheck: `No vulnerabilities found.`;
  - gitleaks: `7 commits scanned`, `no leaks found`.
- `task dev` generates `deploy/.env` (mode 600, random password), and both
  containers reach `healthy`.
- The architecture lint rules fire. A probe file importing `os` in
  `internal/domain` is rejected by depguard; one using `http.DefaultClient` in
  `internal/service` is rejected by forbidigo.
- Secrets never leak: `TestConfig_NeverPrintsSecrets` (fmt, JSON, slog),
  `TestLoad_ErrorsNeverRevealTheDatabaseURL`,
  `TestOpen_InvalidURLNeverRevealsThePassword`, and
  `TestAccessLog_CarriesTheRequestIDAndNoPersonalData`.

### Deliverables

- Go module `github.com/veHRz/MagpieMail-Server`; target layout with a `doc.go`
  per package.
- Taskfile; golangci-lint and gofumpt config; pinned tools.
- `magpie` binary:
  - `serve`;
  - `worker`, `migrate` and `admin` as placeholders;
  - hidden `healthcheck`.
- Configuration (env + YAML) with strict validation; `config.example.yaml`.
- JSON logs with request IDs; `/healthz`, `/readyz`; graceful shutdown.
- Multi-stage distroless Dockerfile; `deploy/docker-compose.yml` with PostgreSQL 18.
- CI workflow (check, integration, audit, image smoke test); Renovate config.
- ADRs 0001 to 0022, README, CHANGELOG, LICENSE (MIT).

### Deviations from the plan

| Plan | Done | Why |
| --- | --- | --- |
| `internal/sync`, `internal/transport/http` | `internal/mailsync`, `internal/transport/httpapi` | Avoid shadowing `sync` and `net/http` ([ADR 0018](adr/0018-package-layout.md)) |
| Subcommands `serve`, `worker`, `migrate`, `admin` | Plus a hidden `healthcheck` | The distroless image has no shell nor curl for `HEALTHCHECK` ([ADR 0017](adr/0017-command-line.md)) |
| Contract first | `/healthz` and `/readyz` before the contract | They are ops probes outside `/api/v1`; S1 adds them to the contract under a `health` tag ([ADR 0020](adr/0020-health-probes.md)) |
| — | Renovate config added | Required by the overview's shared rules, not listed in S0 tasks |
