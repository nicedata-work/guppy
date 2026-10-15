# guppy update

## Synopsis

Download the latest (or a pinned) release, verify its checksum, and
atomically replace the running binary. The previous binary is kept as a
backup until the new one is in place, and restored if installation fails.

Updates the guppy executable only; installed templates are reconciled
with "guppy template update".

## Usage

```
guppy update [flags]
```

## Examples

```
guppy update
  guppy update --version 1.2.3
```

## Flags

| Flag | Description |
|------|-------------|
| --version | update to this exact version instead of latest |


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

