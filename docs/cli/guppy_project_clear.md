# guppy project clear

## Synopsis

Clear the global project index: every known project is forgotten and
the shellhook's seen log is truncated.

The index is only a cache of where .guppy/project.toml markers have
been seen — clearing it never touches the projects themselves, and any
project you scaffold or enter is re-recorded automatically.

## Usage

```
guppy project clear
```

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

