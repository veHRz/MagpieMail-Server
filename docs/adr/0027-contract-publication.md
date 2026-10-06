# 0027. Publish the contract as GitHub releases on api-v* tags

- Status: Accepted
- Date: 2026-10-07
- Deciders: veHRz

## Context

The client repository embeds a frozen copy of the contract (`api/openapi.yaml`
and `api/VERSION`) and needs a stable place to fetch each version. S1 asks for
generated HTML documentation, an `api-v0.1.0` tag and a reusable publication
script.

## Decision

- The contract version is `info.version` of `api/openapi.yaml` (semver), the
  single source; `GET /api/v1/info` returns it as `apiVersion`.
- `task contract:release` (`scripts/release-contract.sh`) runs only on an
  up-to-date, clean `main`. It refuses an existing tag, lints the contract, then
  creates and pushes the annotated tag `api-vX.Y.Z`.
- The tag triggers `.github/workflows/contract.yml`. The workflow checks that the
  tag matches `info.version`, lints, builds the assets
  (`scripts/build-contract.sh`) and creates a GitHub release with:
  - `openapi.yaml`, bundled into one file;
  - `VERSION`;
  - `api-reference.tar.gz`, the HTML reference;
  - `SHA256SUMS`.
- The HTML reference works offline and loads nothing from the Internet:
  - Google Fonts are disabled;
  - the Redoc script Redocly would fetch from its CDN is shipped next to the page,
    downloaded from the npm registry and checked against its published integrity;
  - the build fails if the page still references an external resource.
- CI builds these assets on every change, so a tooling change breaks the build
  rather than a release.
- A tag is created after the phase's pull request is merged, never from a
  phase branch.

## Consequences

The client fetches `openapi.yaml` and `VERSION` from the release of the tag it
targets. GitHub Pages is not used. The release step uses the `gh` CLI, present on
GitHub runners; a Forgejo setup would replace that one step.
