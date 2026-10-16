# guppy template refresh

## Synopsis

Drop the registry cache: cached tarball extractions and git-registry
checkouts under the guppy cache directory are removed, so the next
install re-downloads from the registry.

The installed templates themselves are untouched — refresh only clears
the download cache. Offline-safe by definition: nothing is fetched.
Reconciliation is a different verb: "guppy template update".

## Usage

```
guppy template refresh
```

## Examples

```
guppy template refresh
  guppy template update   # re-fetch with the cache cold
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

