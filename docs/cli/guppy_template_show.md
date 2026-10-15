# guppy template show

## Synopsis

Show what a template will actually be written as: the resolved extends
chain, the merged [vars] and [defaults], and the merged payload file
list (parents overlaid, child wins whole-file). --contents dumps the
merged payload files verbatim.

## Usage

```
guppy template show <id|PATH> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| --contents | also print the merged payload file contents |


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

