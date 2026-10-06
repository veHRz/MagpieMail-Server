#!/usr/bin/env bash
# Installs the pinned, non-Go-module developer tools into ./.bin.
# Every download is verified against the checksum file published with the release.
# Versions are tracked by Renovate through the "renovate:" comments below.
set -euo pipefail

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION="2.14.0"
# renovate: datasource=github-releases depName=gitleaks/gitleaks
GITLEAKS_VERSION="8.30.1"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${ROOT_DIR}/.bin"
mkdir -p "${BIN_DIR}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

# download_verified URL CHECKSUMS_URL FILE: downloads FILE and checks it against CHECKSUMS_URL.
download_verified() {
  local url="$1" sums_url="$2" file="$3"
  curl -fsSL --retry 3 -o "${tmp}/${file}" "${url}"
  curl -fsSL --retry 3 -o "${tmp}/checksums.txt" "${sums_url}"
  (cd "${tmp}" && grep " ${file}\$" checksums.txt | sha256sum -c --status -) || {
    echo "checksum mismatch for ${file}" >&2
    exit 1
  }
}

installed_version() {
  "$1" "$2" 2>/dev/null | head -n1 || true
}

if [[ "$(installed_version "${BIN_DIR}/golangci-lint" --version)" != *" ${GOLANGCI_LINT_VERSION} "* ]]; then
  name="golangci-lint-${GOLANGCI_LINT_VERSION}-${os}-${arch}"
  base="https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}"
  download_verified "${base}/${name}.tar.gz" "${base}/golangci-lint-${GOLANGCI_LINT_VERSION}-checksums.txt" "${name}.tar.gz"
  tar -xzf "${tmp}/${name}.tar.gz" -C "${tmp}" "${name}/golangci-lint"
  install -m 0755 "${tmp}/${name}/golangci-lint" "${BIN_DIR}/golangci-lint"
  echo "installed golangci-lint ${GOLANGCI_LINT_VERSION}"
fi

if [[ "$(installed_version "${BIN_DIR}/gitleaks" version)" != "${GITLEAKS_VERSION}" ]]; then
  gl_arch="${arch/amd64/x64}"
  name="gitleaks_${GITLEAKS_VERSION}_${os}_${gl_arch}.tar.gz"
  base="https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}"
  download_verified "${base}/${name}" "${base}/gitleaks_${GITLEAKS_VERSION}_checksums.txt" "${name}"
  tar -xzf "${tmp}/${name}" -C "${tmp}" gitleaks
  install -m 0755 "${tmp}/gitleaks" "${BIN_DIR}/gitleaks"
  echo "installed gitleaks ${GITLEAKS_VERSION}"
fi
