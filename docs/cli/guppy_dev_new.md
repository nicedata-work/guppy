# guppy dev new

## Synopsis

Create a minimal template shell: a guppy.toml (id, version, kind,
language, one required "name" var) and a template/ payload with a
starter README.

DIR defaults to the template id (created in the cwd). An existing
non-empty directory is refused. Then iterate in place:

  guppy dev lint <dir> && guppy dev test <dir>

## Usage

```
guppy dev new [DIR] [flags]
```

## Examples

```
guppy dev new dbt-v2
  guppy dev new my-template --kind lib --lang python
```

## Flags

| Flag | Description |
|------|-------------|
| --description | one-line description for the registry index |
| --id | template id (default: the directory's base name) |
| --kind (default cli) | template kind: cli, service, lib, app, docs, infra |
| --lang (default none) | template language: go, ts, python, rust, none |


## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

