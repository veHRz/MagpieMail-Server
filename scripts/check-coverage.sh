#!/usr/bin/env bash
# Fails when the total statement coverage of a Go cover profile is below a minimum.
# Usage: scripts/check-coverage.sh PROFILE MINIMUM_PERCENT
set -euo pipefail
profile="${1:?usage: check-coverage.sh PROFILE MINIMUM}"
minimum="${2:?usage: check-coverage.sh PROFILE MINIMUM}"
total="$(go tool cover -func="${profile}" | awk '/^total:/ { sub("%", "", $3); print $3 }')"
echo "repositories coverage: ${total} % (minimum ${minimum} %)"
awk -v total="${total}" -v min="${minimum}" 'BEGIN { exit (total + 0 < min + 0) }' || {
  echo "coverage below ${minimum} %" >&2
  exit 1
}
