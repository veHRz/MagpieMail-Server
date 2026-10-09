#!/usr/bin/env bash
# Runs sqlc from its pinned container image (its PostgreSQL parser needs cgo),
# as the calling user, on the repository. Usage: scripts/sqlc.sh <sqlc arguments>
set -euo pipefail

# renovate: datasource=docker depName=sqlc/sqlc
SQLC_IMAGE="sqlc/sqlc:1.31.1@sha256:70f53171d27b2424e9358869975455a6e955a5aa8e58a998a270a6e34e525537"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec docker run --rm --user "$(id -u):$(id -g)" \
  --volume "${ROOT_DIR}:/src" --workdir /src \
  "${SQLC_IMAGE}" "$@"
