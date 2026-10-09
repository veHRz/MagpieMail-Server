# MagpieMail server

MagpieMail is a self-hosted, privacy-first mail aggregation server: several
users, each with several mail accounts (Gmail, Outlook, Proton through Bridge,
any IMAP/SMTP server), read, sorted, searched and sent from one place. The server
holds all the logic and all the data; clients only talk to its API, described
by `api/openapi.yaml`. Nothing leaves the server except through a single,
audited egress module.

> **Status: early development.** Phases S0 (repository foundations), S1 (API
> contract and HTTP skeleton) and S2 (data model and storage) are done: the
> server serves its published contract (`GET /api/v1/info` and the health
> probes) behind a validated middleware chain, on a migrated schema with
> encrypted secrets and deduplicated blob storage.
> Mail features arrive in the next phases; see [docs/PROGRESS.md](docs/PROGRESS.md).

## Quick start

Requirements: Docker with the compose plugin, and [Task](https://taskfile.dev).

```sh
task dev        # creates deploy/.env and deploy/secrets/master.key if missing, then builds and starts
curl http://127.0.0.1:8080/readyz
task dev:down   # stop (task dev:down -- -v also deletes the database volume)
```

Without Task: copy `deploy/.env.example` to `deploy/.env`, set
`MAGPIEMAIL_DB_PASSWORD`, write a master key to `deploy/secrets/master.key`
(`openssl rand -base64 32`, file mode 444 in a mode 700 directory), then run
`docker compose -f deploy/docker-compose.yml up -d --build`.

The master key in `deploy/secrets/master.key` encrypts every stored credential and
blob: **back it up outside the machine**. Without it they cannot be recovered.
Rotate it with `magpie admin rotate-key` (see `magpie admin rotate-key --help`).

The API is published on `127.0.0.1` only. Put a TLS reverse proxy in front of it
to reach it from other machines.

## API

The API is described by [api/openapi.yaml](api/openapi.yaml) (OpenAPI 3.1).
Each published version is a GitHub release tagged `api-vX.Y.Z`, with the bundled
contract and an HTML reference that works offline. Errors are RFC 9457 problems,
requests are rate limited, and everything under `/api/v1` requires
authentication unless the contract says otherwise.

## Configuration

Settings come from built-in defaults, an optional YAML file (`--config` or
`MAGPIE_CONFIG`), then `MAGPIE_*` environment variables, each layer overriding
the previous one. `MAGPIE_SERVER__READ_TIMEOUT` sets `server.read_timeout`.
Every setting is documented in [config.example.yaml](config.example.yaml).

Loading is strict: an unknown key or variable, or an invalid value, stops the
server with a message naming each faulty setting and where it came from.

## Commands

The single binary `magpie` provides:

| Command | Purpose |
| --- | --- |
| `magpie serve` | Serve the HTTP API described by `api/openapi.yaml` |
| `magpie worker` | Background jobs (phase S6) |
| `magpie migrate up\|down\|status` | Apply or roll back the database migrations |
| `magpie admin generate-key` | Print a new master key |
| `magpie admin rotate-key` | Rewrap every user key with the current master key |

## Development

Requirements: Go (see `go.mod`), Task, Docker (the contract linter and the
integration tests run in containers), curl.

| Task | What it does |
| --- | --- |
| `task check` | Format check, lint (code, CI workflows, API contract), generated code check, unit tests |
| `task test:integration` | Unit and integration tests against real services (Testcontainers) |
| `task test:coverage` | Repositories' coverage with the integration tests; fails below 80 % |
| `task audit` | Known vulnerabilities (govulncheck) and secrets in the git history (gitleaks) |
| `task smoke` | Build the image, check its size and user, and the stack's readiness time |
| `task gen` | Regenerate the server code from the OpenAPI contract (`task gen:check` verifies it) |
| `task contract:lint` | Lint the contract; part of `task check` |
| `task contract:build` | Build the publishable contract into `dist/api` (bundle, offline HTML reference) |
| `task contract:release` | Tag `main` as `api-vX.Y.Z`, which publishes the contract as a GitHub release |
| `task fmt` | Format the code |
| `task tools` | Install the pinned golangci-lint and gitleaks into `.bin/` |

Work happens one phase at a time, in test-driven style, on `phase/SNN-name`
branches with Conventional Commits. The plan is in [docs/plan/](docs/plan/)
(in French); decisions are recorded in [docs/adr/](docs/adr/).

## Layout

```text
api/                 OpenAPI contract, the source of truth shared with clients
cmd/magpie/          the single binary
internal/            server code; one doc.go per package states its role
  domain/            entities and business rules, no I/O
  egress/            the only way out to the network
  transport/httpapi/ HTTP routing, middlewares, probes
  ...
migrations/          SQL migrations (from phase S2)
deploy/              docker compose stack
sdks/                Python and Rust SDKs (phase S13)
test/                end-to-end tests, fixtures, mail corpus
docs/                plan, ADRs, guides, progress
```

## Licence

[MIT](LICENSE).
