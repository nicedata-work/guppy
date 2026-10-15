# guppy doctor

## Synopsis

Audit a guppy-managed project against its template.

Doctor re-renders the template with the vars and toggles recorded in
.guppy/project.toml, diffs the project tree against that expected file
set, and reports:

  missing      files the template expects but the tree lacks
               (safe to restore — the --fix subset)
  modified     files that differ from the template; may be your edits —
               reported, never overwritten
  unexpected   extra files in the .guppy/ metadata dir
  note         project-level findings: stale pin (the installed template
               is newer than the scaffold), missing shellhook, or an
               unresolvable template

With --all, every project in the global index is audited (batch mode);
deleted projects are pruned from the index first .

## Usage

```
guppy doctor [--fix] [flags]
```

## Examples

```
guppy doctor              # audit the project containing cwd
  guppy doctor --fix        # restore missing scaffolded files
  guppy doctor --all        # audit every indexed project
```

## Flags

| Flag | Description |
|------|-------------|
| --all | audit every project in the global index |
| --fix | restore missing scaffolded files (never touches modified files) |


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

