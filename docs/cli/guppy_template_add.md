# guppy template add

## Synopsis

Install a template. Sources:

  <id>                       latest version from the registry
  registry:<id>[@<version>]  version-pinned registry tarball (checksum-verified)
  registry:<name>:<id>[@<v>] same, from the named registry (see
                             [template.registries] in guppy.toml)
  git:<url>[@<ref>]          git checkout, pinned to the resolved commit SHA
  ./path                     a local template directory (copied into scope)
  community:<source>         any of the above into the community scope —
                             requires review + confirm before first use

User-scope installs are trusted immediately; community installs print
the manifest, file list, and (for upgrades) the diff, and require an
interactive confirmation. In non-interactive contexts pass --yes.

## Usage

```
guppy template add <source> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| --yes | skip the interactive review confirmation (CI) |


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

