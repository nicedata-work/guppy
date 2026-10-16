# guppy uninstall

## Synopsis

Remove the guppy binary and the shell-hook block the installer added to
your rc files. User settings (~/.config/guppy) are never touched; pass
--purge to additionally remove data (~/.local/share/guppy) and cache
(~/.cache/guppy).

## Usage

```
guppy uninstall [flags]
```

## Examples

```
guppy uninstall --yes --purge
```

## Flags

| Flag | Description |
|------|-------------|
| --purge | also remove data and cache dirs (never settings) |
| --yes | do not ask for confirmation |


## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

