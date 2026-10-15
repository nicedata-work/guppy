# guppy template search

## Synopsis

Search the configured registries (named team registries first, then
the default) for templates carrying the guppy-template keyword. With no
query, lists every indexed template. The registry is the remote catalog —
search is never local. Install a hit with "guppy template add <id>"
(first-party) or "guppy template add community:<id>" (community,
review-gated).

## Usage

```
guppy template search [query]
```

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

