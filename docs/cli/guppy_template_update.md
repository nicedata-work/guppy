# guppy template update

## Synopsis

Reconcile installed templates with their sources — all of them by
default, or one template when an id is given: registry templates move
to the newest indexed version, git templates re-resolve their ref to
the newest commit (the ref itself never moves). Local-source templates
are loaded in place and have nothing to update.

Community-scope updates show the diff against the installed version and
require review + confirm (--yes in CI); first-party updates apply
directly.

Related: "guppy update ." moves a project to its template's latest;
"guppy template refresh" drops the download cache instead.

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
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

