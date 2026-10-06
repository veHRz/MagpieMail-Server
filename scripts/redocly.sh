#!/usr/bin/env bash
# Runs Redocly CLI from its pinned container image, as the calling user, on the
# repository mounted at /spec. Usage: scripts/redocly.sh <redocly arguments>
set -euo pipefail

# renovate: datasource=docker depName=redocly/cli
REDOCLY_IMAGE="redocly/cli:2.59.0@sha256:7d59c058210751c94409404c2d63506524f96d7164e6b29cdb660120ff418461"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec docker run --rm --user "$(id -u):$(id -g)" \
  --env REDOCLY_TELEMETRY=off --env REDOCLY_SUPPRESS_UPDATE_NOTICE=true \
  --volume "${ROOT_DIR}:/spec" --workdir /spec \
  "${REDOCLY_IMAGE}" "$@"
