# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `magpie` binary with the `serve` command, and `worker`, `migrate` and `admin`
  placeholders for later phases.
- Configuration from defaults, a YAML file and `MAGPIE_*` environment variables,
  with strict validation that names every faulty setting and its origin;
  commented `config.example.yaml`.
- Structured JSON logs with UTC timestamps and a request ID (UUIDv7, or a
  well-formed `X-Request-ID` from a reverse proxy) on every request record.
- `/healthz` (liveness) and `/readyz` (PostgreSQL reachable) probes; graceful
  shutdown on SIGTERM and SIGINT.
- Distroless, non-root container image (about 18 MB) and a docker compose stack
  with PostgreSQL 18.
- CI: format, lint, unit tests with the race detector, integration tests,
  govulncheck, gitleaks, image build and stack smoke test. Renovate
  configuration.
- Architecture decision records 0001 to 0022, README, MIT licence.
