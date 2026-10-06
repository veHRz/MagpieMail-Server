#!/usr/bin/env bash
# Prints the contract version: info.version of api/openapi.yaml.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="$(awk '/^info:/ {in_info = 1; next} in_info && /^[^ ]/ {exit} in_info && /^  version:/ {print $2; exit}' "${ROOT_DIR}/api/openapi.yaml")"
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "api/openapi.yaml: info.version '${version}' is not X.Y.Z" >&2
  exit 1
fi
printf '%s\n' "${version}"
