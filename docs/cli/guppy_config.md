# guppy config

## Synopsis

View and set guppy's global configuration.

Values resolve through the normal precedence chain — defaults, the
settings file (--config or <config>/guppy/guppy.toml), GUPPY_* env
vars, flags — so get/list always show what commands actually see, and
set writes the settings file (comments preserved). Every key has a
GUPPY_* env equivalent; see "guppy config list".

## Usage

```
guppy config
```

## Examples

```
guppy config get template.registry
  guppy config set defaults.linters false
  guppy config set template.dirs "~/templates,/opt/team-templates"
  guppy config list
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


## See also

* [guppy config get](guppy_config_get.md)	Print one config value (resolved)
* [guppy config list](guppy_config_list.md)	List every config key with its resolved value
* [guppy config set](guppy_config_set.md)	Set a config value in the settings file
