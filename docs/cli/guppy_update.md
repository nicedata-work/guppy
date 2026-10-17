# guppy update

## Synopsis

Update things, in three composable forms:

  guppy update        update the guppy executable only
  guppy update --all  update the executable and all installed templates
  guppy update .      update the current project to its template's
                      latest version (requires .guppy/project.toml)

A PATH form re-renders the project's recorded vars and toggles against
the newest template version and merges the result (existing files are
never clobbered — they are kept and reported). Installed templates are
reconciled with "guppy template update"; a project update always uses
what is installed locally after that.

## Usage

```
guppy update [PATH] [flags]
```

## Examples

```
guppy update
  guppy update --all
  guppy update .
```

## Flags

| Flag | Description |
|------|-------------|
| --all | also update all installed templates (executable update + guppy template update) |
| --version | update to this exact version instead of latest |


## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

