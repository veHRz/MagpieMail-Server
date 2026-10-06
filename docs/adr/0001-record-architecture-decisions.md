# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

MagpieMail is built phase by phase from a plan (`docs/plan/`), partly by an AI
agent, with a human review at the end of each phase. Decisions taken along the
way must survive the conversation that produced them.

## Decision

Every non-trivial decision is recorded as an Architecture Decision Record in
`docs/adr/NNNN-title.md`, using the short format of `0000-template.md`
(context, decision, consequences, alternatives). ADRs are written in English,
like everything else committed. An ADR is never rewritten once accepted: a new
ADR supersedes it.

The overview's decisions that only concern the client (Flutter, HTML rendering,
client platforms, offline mode) are recorded in the client repository.

## Consequences

Reviewers can check a phase against its decisions; future phases can find why
the code is the way it is.
