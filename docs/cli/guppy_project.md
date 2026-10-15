# guppy project

## Synopsis

Run a project verb — the project-scoped face of guppy, shimmed as "gp".

guppy project walks up to the nearest .guppy/project.toml (the marker of
a guppy-managed project) and dispatches the verb through that
project's [gp] command map. Verbs execute from the project root through
direnv when the project is direnv-wired (the pinned toolchain
environment); without direnv they run directly — Bazel pins
(.bazelversion) still give version hermeticity.

An unknown verb prints the resolved command map. "doctor" is built in
(project scope) and never appears in the map.

## Usage

```
guppy project [VERB] [ARGS...]
```

## Examples

```
gp build            # inside a project (direnv shim)
  guppy project test  # the explicit fallback, works without direnv
  guppy --directory path/to/proj project lint
```

## Global flags

| Flag | Description |
|------|-------------|
| --color (default auto) | color output: auto, always, never |
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --no-progress | disable progress output |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

