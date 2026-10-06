# 0008. Local AI by default, external providers by explicit choice

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

AI features read mail content. Sending it to a third party is a privacy decision.

## Decision

The default AI provider is a small local model served by Ollama (compose profile
`ai`). OpenAI-compatible and Anthropic adapters exist; an external provider is
used only if the admin allows it and the user consents. A "CLI" adapter driving
locally installed coding agents stays experimental and disabled by default.

## Consequences

Nothing leaves the server for AI until the admin decides it. Mail content is
untrusted input for the model (prompt injection), so AI actions are restricted
to an allow-list (phase S14).

## Alternatives considered

External provider only: better quality, but contrary to the privacy goal.
