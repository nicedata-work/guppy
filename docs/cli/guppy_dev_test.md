# guppy dev test

## Synopsis

Render the template once per case — the implicit "default" case (var
defaults + default toggles) plus every [[case]] in tests.toml beside the
manifest — and compare the FileSet against golden snapshots under
testdata/snapshot/<case>/. Snapshots are updated with -u.

Vars without a value render as "example-<var>" so snapshots are
deterministic. Exit code 1 on any mismatch — this is what catches a
corrupted built-in.

## Usage

```
guppy dev test [PATH|TEMPLATE] [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| -u, --update | rewrite golden snapshots instead of comparing |


## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

