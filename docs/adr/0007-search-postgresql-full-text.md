# 0007. Search with PostgreSQL full-text and trigrams

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The target is a few users and a few hundred thousand messages per instance.

## Decision

Search uses PostgreSQL full-text search (`english` and `simple` configurations
with `unaccent`) and trigrams, behind a `SearchIndex` interface.

## Consequences

No extra service. If measurements justify it, Meilisearch or OpenSearch can be
plugged in behind the interface without touching callers.

## Alternatives considered

Elasticsearch, OpenSearch or Meilisearch from the start: more power than needed,
and one more service.
