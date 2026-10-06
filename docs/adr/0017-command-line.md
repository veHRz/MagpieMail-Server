# 0017. Single binary with cobra subcommands

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

The plan calls for one binary `magpie` with `serve`, `worker`, `migrate` and
`admin` subcommands; `admin` will grow nested commands
(`admin create-user --admin`, `admin rotate-key`).

## Decision

- The command line uses cobra (without viper, see ADR 0016). Shell completion is
  disabled: the binary mostly runs in a container without a shell.
- `cli.Run(ctx, args, env, stdout, stderr) int` takes its environment and
  streams as parameters, so commands are tested in-process without global state.
- Logs go to stderr, keeping stdout for command output (an invitation link, a
  generated password).
- Commands of later phases exist as placeholders that exit with status 1 and
  "not implemented yet".
- A hidden `healthcheck` command probes the local `/readyz`, because the
  distroless image has neither shell nor curl.

## Consequences

A configuration error before the logger exists is printed as plain text on
stderr; once the logger exists, errors are logged as JSON records.
