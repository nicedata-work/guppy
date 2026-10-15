# Environment variables

Every guppy option has a `GUPPY_*` environment equivalent, following the
`UV_*` convention. Precedence: defaults < `guppy.toml` < env < flags.

| Variable | Equivalent | Description |
|---|---|---|
| `GUPPY_TEMPLATE` | `--template` | template id for `guppy new` |
| `GUPPY_OFFLINE` | `--offline` | disable all network access |
| `GUPPY_INSTALL_DIR` | (installer) | install directory (default `~/.local/bin`) |
| `GUPPY_NO_MODIFY_PATH` | `--no-modify-path` | installer: don't edit shell rc files |
| `GUPPY_CONFIG` | `--config` | path to the settings file |
| `GUPPY_DIRECTORY` | `--directory` | run as if started in this directory |
| `GUPPY_NO_PROGRESS` | `--no-progress` | disable progress output |
| `GUPPY_COLOR` | `--color` | `auto`, `always`, or `never` |
| `GUPPY_REGISTRY_TOKEN` | — | bearer token for the default registry (HTTP registries only — git registries use your git credentials) |
| `GUPPY_REGISTRY_<NAME>_TOKEN` | — | bearer token for the HTTP-served registry `<name>` (dashes → underscores), sent as `Authorization: Bearer`; git registries need no token |

The standard `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_CACHE_HOME`
variables relocate guppy's settings, data, and cache directories — see
[Paths and storage](paths.md).

## Exit codes

Exit codes are uniform across subcommands:

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | lint/render errors |
| `2` | unresolved input (ambiguous template, missing var in non-interactive mode) |
| `3` | network/registry failure |
| `130` | interrupted by SIGINT/SIGTERM (partial work rolled back) |
