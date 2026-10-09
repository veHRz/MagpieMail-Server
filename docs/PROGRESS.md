# Progress

Phases come from [docs/plan/01-server-plan.md](plan/01-server-plan.md). A phase
is done only when every validation criterion is proven. Skipping a phase must be
recorded here.

| Phase | Name | Depends on | Status | Branch |
| --- | --- | --- | --- | --- |
| S0 | Repository foundations | — | Done ([PR #1](https://github.com/veHRz/MagpieMail-Server/pull/1)) | `phase/S00-foundations` |
| S1 | API contract and HTTP skeleton | S0 | Done ([PR #5](https://github.com/veHRz/MagpieMail-Server/pull/5)), contract [api-v0.1.0](https://github.com/veHRz/MagpieMail-Server/releases/tag/api-v0.1.0) | `phase/S01-api-contract` |
| S2 | Data model and storage | S0 | Done, in review ([PR #8](https://github.com/veHRz/MagpieMail-Server/pull/8)) | `phase/S02-data-storage` |
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

- [x] **`task check` passes locally and in CI.**
  - Local: proven on 2026-10-06. `task check` exits 0. It runs:
    - `golangci-lint fmt --diff` and `go mod tidy -diff` (no diff);
    - `golangci-lint run` (`0 issues.`);
    - `actionlint`;
    - `go test ./...`, with every package `ok`.
  - The unit tests also pass with `-race`: `go test -race -count=2 ./...` in
    `golang:1.27.1-trixie`. The development machine has no C compiler; CI has one.
  - CI: proven on 2026-10-06. [Run 37535963146](https://github.com/veHRz/MagpieMail-Server/actions/runs/37535963146)
    of PR #1 is green on all four jobs. The `check` job runs `go test -race ./...`
    (every package `ok`) after `0 issues.` from golangci-lint.
- [x] **After `docker compose up`, `/readyz` answers 200 in less than 10 seconds.**
  - `task smoke` (`scripts/smoke.sh`) starts the compose stack on an empty
    database volume (so PostgreSQL's first-time initialization is included), with
    images already built and pulled.
  - It polls `/readyz` until the first 200:
    `PASS  /readyz answered 200 5461 ms after docker compose up (fresh volume)`.
  - Five runs on the development machine: 5384, 5348, 5431, 5405 and 5461 ms.
  - In CI (run 37535963146): `/readyz answered 200 3165 ms`, and `curl` on the
    published port `127.0.0.1:18080` got `{"status":"ok"}`.
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

## S1 — API contract and HTTP skeleton

Goal: the OpenAPI contract exists and is checked in CI, and every request goes
through a common chain (errors, limits, security) ready to receive the domains.

### Validation criteria

- [x] **The contract lint passes without any warning.**
  - `task contract:lint` (`scripts/lint-contract.sh`):
    `Woohoo! Your API description is valid. 🎉`.
  - `redocly.yaml` extends `recommended-strict`, where every finding is an error,
    so a warning would fail the build.
  - 8 findings are explicitly ignored in `.redocly.lint-ignore.yaml`, all
    `no-unused-components`: shared building blocks that the plan requires in S1
    (pagination, event envelope, standard error responses) and that no operation
    uses yet.
  - The 9 house rules are proven to fire: `house rules: all 9 rules fire on the
    broken fixture`.
- [x] **A test checks that every served route exists in the contract.**
  - `TestRouter_ServesExactlyTheContract` walks the production router: every
    served route is in the contract, and every contract operation is served.
  - `TestContract_IsAValidOpenAPIDocument` validates the embedded contract.
  - Every response of the HTTP tests is validated against the contract by
    `contracttest.Serve`. `TestCheck_RejectsNonConformingResponses` proves that
    an undeclared status, content type or body shape fails.
- [x] **400, 401, 404, 413, 429 and 500 responses are all `application/problem+json` (tests).**
  - Each test below checks the content type, `type`, `title`, `status` and
    `requestId`, and validates the response against the contract.
  - On the production router: `TestProblem_404_UnknownRoute`,
    `TestProblem_405_WrongMethod` (also checks `Allow`),
    `TestProblem_429_RateLimitedWithRetryAfter`, and the `production router` case
    of `TestProblem_413_BodyTooLarge`.
  - On test operations mounted on the same middleware chain
    (`internal/transport/httpapi/testdata/chain.yaml`), because the real contract
    has no operation with input or authentication yet:
    - `TestProblem_400_InvalidParameter`, `TestProblem_400_InvalidBody` (3 cases);
    - `TestProblem_401_MissingOrInvalidCredentials`, which also checks
      `WWW-Authenticate`, and `TestAuthentication_*`;
    - `TestProblem_413_BodyTooLarge`, with a declared length and with a streamed
      body;
    - `TestProblem_500_PanicIsRecovered`.
- [x] **Exceeding the rate limit returns 429 with a `Retry-After` header.**
  - `TestProblem_429_RateLimitedWithRetryAfter`: 2 requests pass, the 3rd gets
    429 with `Retry-After: 2` (one token every 2 seconds).
  - Also tested:
    - `TestRateLimit_EachClientAddressHasItsOwnBucket`;
    - `TestRateLimit_PerUserAcrossAddresses`;
    - `TestRateLimit_ProbesAreExempt`;
    - `TestRateLimit_TrustedProxyRevealsTheClientAddress`;
    - `TestRateLimit_UntrustedForwardedForIsIgnored`.
- [x] **CI run of this phase.** [Run 37541033842](https://github.com/veHRz/MagpieMail-Server/actions/runs/37541033842)
  of PR #5 is green on all four jobs. The `check` job (`0 issues.`, contract valid,
  `house rules: all 9 rules fire on the broken fixture`, `go test -race ./...`)
  also builds the contract. The `image` job reports `/readyz answered 200 3041 ms`.

### Also verified

- `task check` exits 0. It runs formatting, `go mod tidy`, golangci-lint
  (`0 issues.`), actionlint, `gen:check`, the contract lint and the unit tests.
- `go test -race -count=2 ./...` (in `golang:1.27.1-trixie`): every package `ok`.
- `task test:integration`: every package `ok`.
- `task gen:check`:
  - passes on the committed code;
  - fails with `oapi.gen.go is stale; run 'task gen'` after a contract change
    without regeneration;
  - never modifies the repository.
- `task smoke`:
  - image of 20 MB (20,406,751 bytes), uid 65532;
  - `/readyz` answers 200 5413 ms after `docker compose up`.
- `task contract:build` builds `openapi.yaml`, `VERSION`, `api-reference.tar.gz`
  and `SHA256SUMS`. The HTML reference references no external resource; the
  script checks it.
- `scripts/release-contract.sh --dry-run` refuses to run outside `main`.
- The real binary:
  - `GET /api/v1/info` returns `{"apiVersion":"0.1.0","features":{"ai":false,"documents":false,"webPush":false}}`;
  - an unknown path returns a 404 problem;
  - `DELETE /api/v1/info` returns 405 with `Allow: GET`.

### Deliverables

- Contract v0.1.0 (`api/openapi.yaml`), lint configuration and house rules.
- Generated server (`internal/transport/httpapi/oapi`), `task gen` and `task gen:check`.
- Middlewares:
  - panic recovery, security headers, CORS;
  - rate limits per address and per user;
  - body limit;
  - authentication hook;
  - contract validation;
  - RFC 9457 problems.
- API documentation: offline HTML reference built by `task contract:build` and
  published with each release.
- Reusable publication: `task contract:release` and `.github/workflows/contract.yml`.
- ADRs 0023 to 0028.

### Publication

- [Release api-v0.1.0](https://github.com/veHRz/MagpieMail-Server/releases/tag/api-v0.1.0),
  published by `task contract:release` on `main` after the merge. Its 4 assets
  were downloaded and checked: `sha256sum -c SHA256SUMS` OK, `VERSION` is
  `0.1.0`, `openapi.yaml` is identical to a local bundle, and the HTML reference
  loads no external resource.

## S2 — Data model and storage

Goal: all MVP data has a migratable schema, files have deduplicated storage,
and secrets are encrypted at rest; all of it tested on a real PostgreSQL.

### Validation criteria

- [x] **Every migration goes up and down on an empty database (integration test).**
  - `TestIntegration_MigrationsGoUpAndDownOnAnEmptyDatabase`, on PostgreSQL 18:
    1. it applies every migration, and the 22 MVP tables exist;
    2. it rolls every migration back, leaving only `goose_db_version`;
    3. it applies them again.
  - `TestIntegration_MigrateUpStatusDown` drives the same cycle through `magpie migrate`.
- [x] **Repositories are covered above 80 % by tests on real PostgreSQL.**
  - `task test:coverage`: `repositories coverage: 88.3 % (minimum 80 %)`.
  - CI enforces it with the integration tests.
- [x] **Writing the same file twice creates one blob; an orphan blob disappears at the purge.**
  - `TestIntegration_SameContentTwiceIsStoredOnce`: two writes return one blob
    record, and exactly one object exists on disk.
  - `TestIntegration_OrphanBlobDisappearsAtPurge`: within the grace period
    nothing is purged; past it, the orphan's record and object are gone, and the
    referenced blob stays.
  - Also `TestIntegration_ReleasedBlobIsPurged`.
- [x] **No secret is readable in clear text in the raw columns (test).**
  - `TestIntegration_NoSecretIsReadableInRawColumns` stores account credentials
    through the repository, then searches every column of every table for the
    secret, as text and as hex bytes: 0 hits.
  - Also tested:
    - `TestIntegration_CiphertextMovedToAnotherAccountDoesNotOpen`;
    - `TestIntegration_EncryptedBlobsAreUnreadableOnDisk`;
    - `TestConfig_NeverPrintsMasterKeys`;
    - `TestLoad_InvalidSecuritySettingsNeverRevealKeys`.
- [x] **Master key rotation re-encrypts everything without loss (test).**
  - `TestIntegration_MasterKeyRotationLosesNothing`:
    - 5 users with encrypted credentials;
    - a rotation to a new key rewraps 5 user keys;
    - with only the new key, every secret reads back identical;
    - the old key alone opens nothing.
  - Also tested:
    - `TestIntegration_RotationRefusesKeysItCannotOpen`, where a failed rotation
      changes nothing;
    - `TestIntegration_RotateKeyCommand`: `magpie admin rotate-key` end to end,
      audit event included.

### Also verified

- `task check` exits 0. It includes `gen:check`, which now also runs
  `sqlc diff`; it fails on a query changed without `task gen`.
- `task test:integration`: every package `ok`.
- `go test -race -count=2 ./...` (in the pinned `golang` image): every package `ok`.
- `task audit`: `No vulnerabilities found.`, `no leaks found` on 26 commits.
  - On the first run, govulncheck reported 9 vulnerabilities of the Go 1.27.1
    standard library: the project moved to Go 1.27.2.
  - gitleaks flagged two deliberately invalid test keys: the test now builds
    them at run time, and the findings are recorded in `.gitleaksignore`.
- `task smoke`:
  - image of 21 MB, uid 65532;
  - `/readyz` answers 200 6086 ms after `docker compose up`, the one-shot
    migration included;
  - the server reads its master key from a Docker secret file.
- A fresh `blobs` volume is owned by `65532:65532`.

- CI: [run 37994909038](https://github.com/veHRz/MagpieMail-Server/actions/runs/37994909038)
  of PR #8 is green on all four jobs:
  - `0 issues.`, `house rules: all 9 rules fire on the broken fixture`,
    `go test -race ./...`;
  - `repositories coverage: 88.3 % (minimum 80 %)`;
  - `/readyz answered 200 3650 ms`, and the published port answers;
  - `No vulnerabilities found.`, `no leaks found`.
- The first runs failed on Docker Hub: the anonymous pull quota was exhausted,
  then a Docker Hub partial outage. CI now logs in to Docker Hub with repository
  secrets (`Login Succeeded` in the 3 jobs that pull images) and falls back to
  anonymous pulls when they are absent.

### Deliverables

- Migrations (`migrations/`) and `magpie migrate up|down|status`.
- Repositories (`internal/store/postgres`) over sqlc-generated queries:
  - users;
  - user keys;
  - providers;
  - accounts;
  - blob records;
  - audit log.
- Blob storage:
  - `internal/store/blobstore`: the interface, the local disk backend and the
    conformance suite;
  - `internal/store/blobs`: deduplication, encryption and purge.
- Encryption module (`internal/security/envelope`), `magpie admin generate-key`
  and `magpie admin rotate-key`.
- ADRs 0029 to 0032.

### Deviations from the plan

| Plan | Done | Why |
| --- | --- | --- |
| A repository per aggregate | Schema complete; repositories for users, keys, providers, accounts, blobs and audit log | The others are written with their phase, once their queries are known ([ADR 0029](adr/0029-data-model-conventions.md)) |
| SHA-256 deduplication | Per-user deduplication on keyed HMAC-SHA256 fingerprints | No cross-user confirmation of files, compatible with per-user encryption ([ADR 0030](adr/0030-blob-storage-dedup-and-encryption.md)) |
| S3-compatible storage | Interface and conformance suite only | Needs outbound traffic through `internal/egress`, which does not exist yet ([ADR 0030](adr/0030-blob-storage-dedup-and-encryption.md)) |
| Master key rotation re-encrypts everything | Every user key is rewrapped with the new master key; data keys do not change | Envelope encryption: fast, atomic, no downtime ([ADR 0031](adr/0031-master-key-and-rotation.md)) |
