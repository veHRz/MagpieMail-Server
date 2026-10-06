# 0005. Background jobs with River

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Synchronization, sending, rules, AI, OCR and purges run in the background and
must survive a worker crash.

## Decision

Background jobs use River, a job queue stored in PostgreSQL, run by `magpie worker`.

## Consequences

Jobs and data share transactions, and there is one service fewer to operate and
back up.

## Alternatives considered

Redis with Asynq: proven, but one more stateful service.
