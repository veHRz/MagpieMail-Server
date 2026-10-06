# 0021. Distroless non-root image and a locked-down compose stack

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The image must run without root and weigh less than 50 MB; the reference stack
must be safe by default for a self-hoster.

## Decision

- Multi-stage Dockerfile: a static binary (`CGO_ENABLED=0`, `-trimpath`, stripped)
  on `gcr.io/distroless/static-debian13:nonroot`, `USER 65532:65532` (numeric, so
  that orchestrators can verify non-root). Base images are pinned by digest.
- `HEALTHCHECK` runs `magpie healthcheck` (ADR 0017).
- `deploy/docker-compose.yml`:
  - PostgreSQL sits on an `internal` network with no route to the Internet;
  - the API is published on `127.0.0.1` only (a TLS reverse proxy goes in front);
  - the server container is read-only, drops all capabilities, and sets
    `no-new-privileges`;
  - the database password comes from `deploy/.env`, ignored by git and generated
    by `task dev`;
  - the PostgreSQL volume is mounted at `/var/lib/postgresql`, as required by
    images 18 and later.
- `scripts/smoke.sh` checks the image and the stack. It is run by `task smoke`
  and in CI.

## Consequences

The first image weighs about 18 MB. The readiness measure starts at
`docker compose up` on an empty volume (so it includes PostgreSQL's first
initialization), with images already built and pulled: download time depends on
the network, not on the server.
