# guppy new

## Synopsis

Scaffold a new project from a template.

Without --template, the template is picked from the kind × language axes
(--kind/--lang); ambiguity is an error listing candidates. Every default
axis has a --no-* inverse, and --bare turns all of them off.

An existing non-empty directory is refused; pass --force to merge into it
(existing files are never clobbered — they are kept and reported, and
re-runs are idempotent).

Vars are prompted interactively when they have no value; in non-interactive
contexts pass --var name=value (missing vars are exit code 2).

## Usage

```
guppy new [PATH] [flags]
```

## Examples

```
guppy new my-cli --template go-cli
  guppy new . --kind cli --lang go --var module_path=example.com/x/y
  guppy new existing-dir --force --no-direnv
```

## Flags

| Flag | Description |
|------|-------------|
| --bare | scaffold minimally: turn every default axis off |
| --dry-run | print the files that would be created, write nothing |
| --force | merge into an existing non-empty directory (never clobbers files) |
| --kind | pick the template by kind: cli, service, lib, app, docs, infra |
| --lang | pick the template by language: go, ts, python, rust, none |
| --no-agents-md | disable the agents_md default (no AGENTS.md) |
| --no-bazel | disable the bazel default |
| --no-direnv | disable the direnv default |
| --no-git | disable the git default (no .git init, no git ignores) |
| --no-linters | disable the linters default |
| --no-tests | disable the tests default |
| -t, --template | template to use (id or directory); GUPPY_TEMPLATE |
| -x, --var (default []) | set a template var (key=value, repeatable); GUPPY_VARS |


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

