#!/usr/bin/env bash
# Publishes the API contract: tags main as api-vX.Y.Z, where X.Y.Z is
# info.version of api/openapi.yaml, and pushes the tag. The "Contract release"
# workflow then builds the assets (scripts/build-contract.sh) and creates the
# GitHub release. Bump info.version before releasing a changed contract.
#
# Usage: scripts/release-contract.sh [--dry-run]
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
fail() { echo "release-contract: $*" >&2; exit 1; }

dry_run=false
[[ "${1:-}" == "--dry-run" ]] && dry_run=true

version="$(scripts/contract-version.sh)"
tag="api-v${version}"

[[ "$(git rev-parse --abbrev-ref HEAD)" == "main" ]] || fail "release from main"
git diff --quiet && git diff --cached --quiet || fail "the working tree has uncommitted changes"
git fetch --quiet --tags origin main
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || fail "main differs from origin/main"
if git rev-parse --quiet --verify "refs/tags/${tag}" >/dev/null; then
  fail "${tag} already exists: bump info.version in api/openapi.yaml"
fi

scripts/lint-contract.sh

if ${dry_run}; then
  echo "dry run: would tag $(git rev-parse --short HEAD) as ${tag} and push it"
  exit 0
fi
git tag --annotate "${tag}" --message "API contract ${version}"
git push origin "${tag}"
echo "pushed ${tag}; the Contract release workflow publishes it"
