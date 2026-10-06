#!/usr/bin/env bash
# Smoke-tests a built image and the compose stack. Proves the S0 criteria:
#   - the image runs as a non-root user and weighs less than 50 MB;
#   - an invalid configuration fails startup, naming the faulty key;
#   - after `docker compose up`, /readyz answers 200 in less than 10 seconds
#     (measured from a fresh database volume, so including PostgreSQL's
#     first-time initialization; images are built and pulled beforehand).
#
# Usage: scripts/smoke.sh IMAGE
# Set SMOKE_HOST_CURL=1 when the Docker host's loopback is reachable (CI) to
# also probe the published port with curl.
set -euo pipefail

IMAGE="${1:?usage: scripts/smoke.sh IMAGE}"
MAX_IMAGE_BYTES=50000000
MAX_READY_SECONDS=10
PROJECT="magpiemail-smoke"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export MAGPIEMAIL_IMAGE="${IMAGE}"
export MAGPIEMAIL_HTTP_PORT="${SMOKE_HTTP_PORT:-18080}"
MAGPIEMAIL_DB_PASSWORD="$(openssl rand -hex 16)"
export MAGPIEMAIL_DB_PASSWORD
compose() { docker compose -p "${PROJECT}" -f "${ROOT_DIR}/deploy/docker-compose.yml" "$@"; }

pass() { printf 'PASS  %s\n' "$*"; }
fail() { printf 'FAIL  %s\n' "$*" >&2; exit 1; }

# --- Image ------------------------------------------------------------------

user="$(docker image inspect --format '{{.Config.User}}' "${IMAGE}")"
uid="${user%%:*}"
[[ -n "${uid}" && "${uid}" != "0" && "${uid}" != "root" ]] || fail "image user is '${user}', expected non-root"
pass "image user is ${user} (non-root)"

size="$(docker image inspect --format '{{.Size}}' "${IMAGE}")"
((size < MAX_IMAGE_BYTES)) || fail "image weighs ${size} bytes, limit is ${MAX_IMAGE_BYTES}"
pass "image weighs $((size / 1000000)) MB (${size} bytes < ${MAX_IMAGE_BYTES})"

set +e
output="$(docker run --rm -e MAGPIE_SERVER__LISTEN=nope \
  -e MAGPIE_DATABASE__URL=postgres://magpie:pw@db:5432/magpie "${IMAGE}" serve 2>&1)"
code=$?
set -e
[[ ${code} -ne 0 ]] || fail "serve started with an invalid configuration"
[[ "${output}" == *"server.listen"* ]] || fail "error does not name the faulty key: ${output}"
pass "invalid configuration exits with code ${code}: $(printf '%s' "${output}" | tr '\n' ' ')"

# --- Stack ------------------------------------------------------------------

cleanup() { compose down --volumes --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
cleanup # start from an empty database volume

# Image downloads depend on the network, not on the server: do them before timing.
compose pull --quiet postgres >/dev/null 2>&1 || fail "could not pull the PostgreSQL image"

start="$(date +%s%N)"
compose up --detach --no-build --quiet-pull >/dev/null 2>&1 || {
  compose logs
  fail "docker compose up failed"
}
# The image has no curl: `magpie healthcheck` GETs /readyz and exits 0 on 200.
until compose exec -T magpie /usr/local/bin/magpie healthcheck >/dev/null 2>&1; do
  if (($(date +%s%N) - start > 60 * 1000000000)); then
    compose logs
    fail "/readyz did not answer 200 within 60 s"
  fi
  sleep 0.1
done
elapsed_ms=$((($(date +%s%N) - start) / 1000000))
((elapsed_ms < MAX_READY_SECONDS * 1000)) || fail "/readyz answered 200 after ${elapsed_ms} ms, limit is ${MAX_READY_SECONDS} s"
pass "/readyz answered 200 ${elapsed_ms} ms after docker compose up (fresh volume)"

if [[ "${SMOKE_HOST_CURL:-0}" == "1" ]]; then
  body="$(curl --fail --silent --show-error "http://127.0.0.1:${MAGPIEMAIL_HTTP_PORT}/readyz")"
  pass "published port 127.0.0.1:${MAGPIEMAIL_HTTP_PORT} answers /readyz: ${body}"
fi

process_uid="$(docker top "$(compose ps --quiet magpie)" -o pid,uid | awk 'NR == 2 {print $2}')"
[[ "${process_uid}" =~ ^[0-9]+$ && "${process_uid}" != "0" ]] || fail "server process runs as uid '${process_uid}'"
pass "server process runs as uid ${process_uid}"
