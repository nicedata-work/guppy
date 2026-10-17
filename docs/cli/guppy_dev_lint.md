# guppy dev lint

## Synopsis

Lint a template without rendering it: guppy.toml schema,
extends resolution (unresolvable parents and cycles are errors), and every
{{ }} substitution in the payload (undeclared vars and toggles are errors,
never silently empty output).

PATH is a template directory (default "."), TEMPLATE a resolvable template
id such as a built-in. Exit code 1 if any error-level issue is found.

## Usage

```
guppy dev lint [PATH|TEMPLATE]
```

## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

