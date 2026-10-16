# guppy project jump

## Synopsis

Resolve a guppy-managed project to its root directory.

NAME matches the project directory's base name as recorded in the
global index. With no argument, exactly one known project is required —
ambiguity is an error listing the candidates.

A child process cannot change its parent shell's directory, so jump
prints the path; the shellhook (guppy shell-env) or an explicit
`cd $(guppy project jump NAME)` does the actual cd.

## Usage

```
guppy project jump [NAME]
```

## Examples

```
cd "$(guppy project jump dbt-project)"
  eval "$(guppy shell-env zsh)"   # installs the jump-aware hook
```

## Global flags

| Flag | Description |
|------|-------------|
| --color (default auto) | color output: auto, always, never |
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --no-progress | disable progress output |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

