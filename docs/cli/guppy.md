# guppy

## Synopsis

guppy scaffolds new projects from templates with great defaults —
language- and target-agnostic. Templates are declarative (guppy.toml +
payload); scaffolds produce hermetic dev environments: Bazel toolchains +
direnv, never polluting global state.

Run "guppy <verb>" for global management (templates, configuration,
doctor) or "guppy project <verb>" (shimmed as "gp") inside a
guppy-managed project.

## Usage

```
guppy [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| --color (default auto) | color output: auto, always, never |
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --no-progress | disable progress output |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |


## See also

* [guppy completion](guppy_completion.md)	Generate the autocompletion script for the specified shell
* [guppy doctor](guppy_doctor.md)	Audit a guppy-managed project against its template 
* [guppy init-shell](guppy_init-shell.md)	Emit the shellhook that keeps the project index fresh
* [guppy lint](guppy_lint.md)	Lint a template: manifest, extends chain, and payload substitutions
* [guppy list](guppy_list.md)	List guppy-managed projects on this machine
* [guppy new](guppy_new.md)	Scaffold a new project from a template
* [guppy project](guppy_project.md)	Run a project verb from .guppy/project.toml (shimmed as gp)
* [guppy template](guppy_template.md)	Inspect and manage templates
* [guppy test](guppy_test.md)	Render snapshot tests for a template (golden files, var matrices)
* [guppy uninstall](guppy_uninstall.md)	Remove guppy from this machine
* [guppy update](guppy_update.md)	Update the guppy executable in place
