# 0016. Configuration: defaults, YAML file, environment; strict validation

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Operators configure the server through a file, environment variables (Docker),
or both. A typo must never be silently ignored, and the startup error must say
which setting is wrong.

## Decision

- Layers, lowest to highest precedence: built-in defaults, an optional YAML file
  (`--config` or `MAGPIE_CONFIG`), then environment variables. Merging uses koanf.
- Environment names: `MAGPIE_` + key in upper case, with `__` between levels
  (`MAGPIE_SERVER__READ_TIMEOUT` → `server.read_timeout`). A single `_` stays
  part of the key, so keys may contain underscores.
- The whole `MAGPIE_` namespace belongs to the server: an unknown `MAGPIE_*`
  variable is an error, like an unknown file key. Variables of the compose stack
  therefore use `MAGPIEMAIL_` (`MAGPIEMAIL_DB_PASSWORD`).
- Durations require a unit (`"30s"`); a bare number is rejected rather than read
  as nanoseconds.
- Every problem is reported at once, each naming the key and where the value came
  from (file path or variable name). Error messages never quote secret values.
- Secrets use the `config.Secret` type, which prints `[REDACTED]` through fmt,
  JSON, text marshalling and slog; only `Reveal()` returns the value.
- `config.example.yaml` documents every setting; a test fails when a setting is
  missing from it or when it stops loading.

## Consequences

Startup fails fast with an actionable message. Secrets in files are possible but
discouraged; the Docker-secret (`_FILE`) mechanism arrives with the master key
in phase S2.

## Alternatives considered

Viper: heavier and lenient by default. Single `_` as level separator: ambiguous
with keys that contain underscores.
