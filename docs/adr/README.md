# Architecture Decision Records

New ADRs copy [0000-template.md](0000-template.md). Accepted ADRs are never rewritten; a
new ADR supersedes them.

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-server-language-go.md) | Write the server in Go | Accepted |
| [0003](0003-rest-api-openapi-contract-first.md) | REST API described by OpenAPI 3.1, contract first | Accepted |
| [0004](0004-postgresql-sqlc-goose.md) | PostgreSQL with pgx, sqlc and goose | Accepted |
| [0005](0005-background-jobs-river.md) | Background jobs with River | Accepted |
| [0006](0006-blob-storage.md) | Content-addressed blob storage outside the database | Accepted |
| [0007](0007-search-postgresql-full-text.md) | Search with PostgreSQL full-text and trigrams | Accepted |
| [0008](0008-local-ai-by-default.md) | Local AI by default, external providers by explicit choice | Accepted |
| [0009](0009-ocr-and-text-extraction.md) | OCR with OCRmyPDF and text extraction with Apache Tika | Accepted |
| [0010](0010-hooks-in-isolated-runner.md) | Run user hooks in an isolated hook-runner container | Accepted |
| [0011](0011-ci-github-actions.md) | Continuous integration with GitHub Actions | Accepted |
| [0012](0012-license-mit.md) | MIT licence | Accepted |
| [0013](0013-single-egress-module.md) | A single egress module for all outbound traffic | Accepted |
| [0014](0014-server-master-key-for-secrets.md) | Encrypt secrets with a server master key | Accepted |
| [0015](0015-re2-regular-expressions.md) | User-supplied regular expressions use RE2 | Accepted |
| [0016](0016-configuration.md) | Configuration: defaults, YAML file, environment; strict validation | Accepted |
| [0017](0017-command-line.md) | Single binary with cobra subcommands | Accepted |
| [0018](0018-package-layout.md) | Package names that do not shadow the standard library | Accepted |
| [0019](0019-logging-and-request-ids.md) | Structured logs and request IDs | Accepted |
| [0020](0020-health-probes.md) | Health probes outside the versioned API | Accepted |
| [0021](0021-container-image-and-compose.md) | Distroless non-root image and a locked-down compose stack | Accepted |
| [0022](0022-developer-tooling.md) | Developer tooling and quality gates | Accepted |
| [0023](0023-contract-tooling.md) | Contract tooling: Redocly for linting and publishing, oapi-codegen for the server | Accepted |
| [0024](0024-request-pipeline.md) | One middleware chain, validated against the contract, for every request | Accepted |
| [0025](0025-rate-limiting.md) | In-memory token-bucket rate limiting per address and per user | Accepted |
| [0026](0026-authentication-hook-and-security-schemes.md) | Security schemes, secure by default, and the authentication hook | Accepted |
| [0027](0027-contract-publication.md) | Publish the contract as GitHub releases on api-v* tags | Accepted |
| [0028](0028-public-instance-information.md) | Public instance information without the server build | Accepted |
| [0029](0029-data-model-conventions.md) | Data model conventions and per-user isolation in the schema | Accepted |
| [0030](0030-blob-storage-dedup-and-encryption.md) | Blob storage: per-user deduplication, keyed fingerprints, encryption | Accepted |
| [0031](0031-master-key-and-rotation.md) | Master key supply, per-user root keys and rotation | Accepted |
| [0032](0032-migrations-and-sql-tooling.md) | Migrations with goose, typed queries with sqlc, both from the binary or a container | Accepted |
