#!/usr/bin/env bash
# Lints the API contract and checks that the house rules still work.
#   1. api/openapi.yaml must produce no problem at all (warnings fail too:
#      redocly.yaml extends recommended-strict).
#   2. test/contract/rules-fixture.yaml breaks every house rule once: each rule
#      must report it, so that a rule silently disabled by a config change or a
#      Redocly upgrade is caught.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
redocly() { "${ROOT_DIR}/scripts/redocly.sh" "$@"; }

redocly lint api/openapi.yaml --format=stylish

set +e
fixture_output="$(redocly lint test/contract/rules-fixture.yaml --format=stylish 2>&1)"
fixture_status=$?
set -e
if [[ ${fixture_status} -eq 0 ]]; then
  echo "house rules: the broken fixture passed the lint" >&2
  exit 1
fi

missing=0
for rule in $(grep -oE '^  rule/[a-z0-9-]+' "${ROOT_DIR}/redocly.yaml" | tr -d ' '); do
  if ! grep -q "${rule}" <<<"${fixture_output}"; then
    echo "house rules: ${rule} did not fire on test/contract/rules-fixture.yaml" >&2
    missing=1
  fi
done
if [[ ${missing} -ne 0 ]]; then
  printf '%s\n' "${fixture_output}" >&2
  exit 1
fi
echo "house rules: all $(grep -cE '^  rule/' "${ROOT_DIR}/redocly.yaml") rules fire on the broken fixture"
