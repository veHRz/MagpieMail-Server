#!/usr/bin/env bash
# Builds the published form of the API contract into dist/api:
#   openapi.yaml           the contract, bundled into one file
#   VERSION                its version (X.Y.Z)
#   api-reference.tar.gz   HTML reference: index.html + redoc.standalone.js
#   SHA256SUMS             checksums of the files above
#
# The HTML reference loads nothing from the Internet: no Google Fonts, and the
# Redoc script that Redocly would load from its CDN is shipped next to it,
# downloaded from the npm registry and checked against its published integrity.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="dist/api" # relative to the repository: Redocly sees it at /spec/dist/api
cd "${ROOT_DIR}"
redocly() { "${ROOT_DIR}/scripts/redocly.sh" "$@"; }

version="$("${ROOT_DIR}/scripts/contract-version.sh")"
rm -rf "${OUT}"
mkdir -p "${OUT}/api-reference"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

redocly bundle api/openapi.yaml --output "${OUT}/openapi.yaml" >/dev/null
printf '%s\n' "${version}" >"${OUT}/VERSION"

html="${OUT}/api-reference/index.html"
redocly build-docs api/openapi.yaml --output "${html}" --disableGoogleFont \
  --title "MagpieMail API ${version}" >/dev/null

cdn_url="$(grep -oE 'https://cdn\.redocly\.com/redoc/v[0-9.]+/bundles/redoc\.standalone\.js' "${html}" | head -n1)"
[[ -n "${cdn_url}" ]] || { echo "build-docs no longer references the Redoc CDN script; update this script" >&2; exit 1; }
redoc_version="${cdn_url#*redoc/v}"
redoc_version="${redoc_version%%/*}"

metadata="$(curl -fsSL "https://registry.npmjs.org/redoc/${redoc_version}")"
tarball_url="$(jq -r .dist.tarball <<<"${metadata}")"
integrity="$(jq -r .dist.integrity <<<"${metadata}")"
curl -fsSL -o "${tmp}/redoc.tgz" "${tarball_url}"
actual="sha512-$(openssl dgst -sha512 -binary "${tmp}/redoc.tgz" | base64 | tr -d '\n')"
[[ "${actual}" == "${integrity}" ]] || { echo "redoc ${redoc_version}: integrity mismatch" >&2; exit 1; }
tar -xzf "${tmp}/redoc.tgz" -C "${tmp}" package/bundles/redoc.standalone.js
cp "${tmp}/package/bundles/redoc.standalone.js" "${OUT}/api-reference/redoc.standalone.js"
sed -i.bak "s#${cdn_url}#redoc.standalone.js#g" "${html}" && rm "${html}.bak"

if grep -qE '<(script|link|img|iframe)[^>]+(src|href)="(https?:)?//' "${html}"; then
  echo "${html} still loads an external resource:" >&2
  grep -oE '<(script|link|img|iframe)[^>]+(src|href)="(https?:)?//[^"]+"' "${html}" >&2
  exit 1
fi

tar -czf "${OUT}/api-reference.tar.gz" -C "${OUT}" api-reference
rm -rf "${OUT}/api-reference"
(cd "${OUT}" && sha256sum openapi.yaml VERSION api-reference.tar.gz >SHA256SUMS)
echo "contract ${version} built into ${OUT}:"
ls -l "${OUT}"
