# guppy project

## Synopsis

Create and manage projects, and run project verbs — the project-scoped
face of guppy, shimmed as "gp".

Built-in subcommands cover the project lifecycle: new, list, jump, and
clear. Inside a guppy-managed project, any other verb dispatches
through that project's [gp] command map (.guppy/project.toml), walking
up to the nearest marker first. Built-in names shadow map verbs: a
project defining its own "list" verb must invoke it as
gp's explicit fallback or rename it.

Map verbs execute from the project root through direnv when the
project is direnv-wired (the pinned toolchain environment); without
direnv they run directly — Bazel pins (.bazelversion) still give
version hermeticity. An unknown verb prints the resolved command map.
"doctor" is built in (project scope) and never appears in the map.

## Usage

```
guppy project [VERB] [ARGS...]
```

## Examples

```
guppy project new dbt-project --template dbt-v2
  guppy project list
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
| --interactive | enable interactive prompts |
| --no-progress | disable progress output |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |


## See also

* [guppy project clear](guppy_project_clear.md)	Forget all known projects (clears the global index)
* [guppy project jump](guppy_project_jump.md)	Print a known project's root directory (cd via the shellhook)
* [guppy project list](guppy_project_list.md)	List guppy-managed projects on this machine
* [guppy project new](guppy_project_new.md)	Scaffold a new project from a template (== guppy project new)
