# guppy template update

## Synopsis

Reconcile installed templates with their sources: registry templates
move to the newest indexed version, git templates re-resolve their ref
to the newest commit (the ref itself never moves). Local-source
templates are loaded in place and have nothing to update.

Community-scope updates show the diff against the installed version and
require review + confirm (--yes in CI); first-party updates apply
directly.

## Usage

```
guppy template update [<id>|community:<id>] [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| --all | update every installed template |
| --yes | skip the interactive review confirmation (CI) |


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

