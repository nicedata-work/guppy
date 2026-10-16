# guppy

## Synopsis

guppy scaffolds new projects from templates with great defaults —
language- and target-agnostic. Templates are declarative (guppy.toml +
payload); scaffolds produce hermetic dev environments: Bazel toolchains +
direnv, never polluting global state.

"guppy project" (shimmed as "gp") creates and manages projects and
dispatches project verbs inside a guppy-managed project; "guppy
template" manages installed templates; "guppy dev" is the template
authoring loop; "guppy config" views and sets global configuration.

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
| --interactive | enable interactive prompts |
| --no-progress | disable progress output |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |


## See also

* [guppy completion](guppy_completion.md)	Generate the autocompletion script for the specified shell
* [guppy config](guppy_config.md)	View and set guppy global config
* [guppy dev](guppy_dev.md)	Develop guppy templates (new, list, lint, test, publish)
* [guppy doctor](guppy_doctor.md)	Audit a guppy-managed project against its template 
* [guppy new](guppy_new.md)	Scaffold a new project from a template (== guppy project new)
* [guppy project](guppy_project.md)	Create and manage projects (shimmed as gp)
* [guppy shell-env](guppy_shell-env.md)	Emit the shellhook that keeps the project index fresh
* [guppy template](guppy_template.md)	Inspect and manage templates
* [guppy uninstall](guppy_uninstall.md)	Remove guppy from this machine
* [guppy update](guppy_update.md)	Update guppy, templates, or a project
