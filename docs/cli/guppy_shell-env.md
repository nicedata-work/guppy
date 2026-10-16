# guppy shell-env

## Synopsis

Emit the shellhook that keeps the global project index fresh.

Without --install the hook is printed for eval in your rc file:

    eval "$(guppy shell-env zsh)"

With --install it is written into the rc file as a marked block
(idempotent; `guppy uninstall` removes it). The hook itself is a
stub: one directory comparison per prompt and one append when you enter
a guppy-managed project — it never invokes guppy, so it cannot slow
down or break your shell. Guppy folds its observations into
index.toml whenever a command reads the index (guppy project list,
doctor).

## Usage

```
guppy shell-env [zsh|bash] [flags]
```

## Examples

```
eval "$(guppy shell-env zsh)"
  guppy shell-env bash --install
```

## Flags

| Flag | Description |
|------|-------------|
| --install | write the hook into the rc file (marked block) instead of printing it |


## Global flags

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

