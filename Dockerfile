# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# Base images are pinned by digest; Renovate keeps tags and digests up to date.
FROM --platform=$BUILDPLATFORM golang:1.27.1-trixie@sha256:8f58fd67ea075142d947a60e0caa4317746a55118d312f027793d382c7741734 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w \
        -X github.com/veHRz/MagpieMail-Server/internal/buildinfo.Version=${VERSION} \
        -X github.com/veHRz/MagpieMail-Server/internal/buildinfo.Commit=${COMMIT}" \
      -o /out/magpie ./cmd/magpie

# Static, shell-less runtime image. "nonroot" is uid/gid 65532.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

ARG VERSION=dev
LABEL org.opencontainers.image.title="MagpieMail server" \
      org.opencontainers.image.description="Self-hosted, privacy-first mail aggregation server" \
      org.opencontainers.image.source="https://github.com/veHRz/MagpieMail-Server" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /out/magpie /usr/local/bin/magpie

# Numeric ids let orchestrators verify runAsNonRoot.
USER 65532:65532
EXPOSE 8080

# No shell nor curl in the image: the binary probes its own /readyz.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --start-interval=1s --retries=3 \
  CMD ["/usr/local/bin/magpie", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/magpie"]
CMD ["serve"]
