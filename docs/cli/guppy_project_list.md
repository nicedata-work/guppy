# guppy project list

## Synopsis

List the guppy-managed projects known on this machine.

The list comes from the global index (index.toml), which is maintained
by the shellhook ("guppy shell-env") and pruned on every read: entries
whose project directory or .guppy/project.toml no longer exists are
removed automatically. Projects scaffolded or audited by guppy are
recorded even without the hook; "guppy project clear" forgets them all.

## Usage

```
guppy project list
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

