# guppy dev publish

## Synopsis

Publish a template the way the registry accepts contributions: a
versioned tarball and one index.json entry per version.

Publish runs the same gates consumers' CI runs — guppy dev lint and
guppy dev test — then writes guppy-<id>-<version>.tar.gz (the template
wrapped in its id/ directory) plus its sha256 into --out, and prints
the index.json Entry to append. Uploading is manual by design: the
registry is static files, and first-party contributions are a PR to the
guppy-registry repository.

## Usage

```
guppy dev publish [PATH] [flags]
```

## Examples

```
guppy dev publish ./my-template
  guppy dev publish . --out ~/registry-staging
```

## Flags

| Flag | Description |
|------|-------------|
| --out (default ./dist) | directory for the built tarball |
| --skip-tests | skip the snapshot-test gate (lint always runs) |


## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

