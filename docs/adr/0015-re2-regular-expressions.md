# 0015. User-supplied regular expressions use RE2

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Sorting rules accept regular expressions written by users. A backtracking engine
can be stalled by a catastrophic pattern (ReDoS).

## Decision

User regexes are compiled with Go's `regexp` package (RE2, linear-time
matching), with limits on pattern size and evaluation. Backtracking engines such
as `github.com/dlclark/regexp2` are forbidden by depguard.

## Consequences

Some Perl features (backreferences, lookaround) are unavailable to users; the
rule documentation says so (phase S12).
