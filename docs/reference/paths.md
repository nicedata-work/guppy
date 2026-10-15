# Paths and storage

guppy follows uv's XDG layout, honoured per-OS and overridable by the
standard `XDG_*_HOME` variables on Linux/macOS.

| Kind | Linux / macOS | Windows |
|---|---|---|
| binary | `~/.local/bin/guppy` | `%USERPROFILE%\.local\bin\guppy.exe` |
| settings | `~/.config/guppy/guppy.toml` | `%APPDATA%\guppy\guppy.toml` |
| data | `~/.local/share/guppy/` | `%LOCALAPPDATA%\guppy\` |
| cache | `~/.cache/guppy/` | `%LOCALAPPDATA%\guppy\cache\` |
| per-project | `<project>/.guppy/` | same |

## What lives in data/

| Path | Contents |
|---|---|
| `templates/` | user-scope templates |
| `community/<id>@<version>/` | installed community templates, version-pinned |
| `index.toml` | the global index of managed projects |
| `catalog/` | shellhook state (the append-only `seen.log`) |

## Invariants

- Everything the *user configures* lives in the settings file; everything
  guppy *generates* lives under `data/` or `cache/`.
- Nothing is written to dotfile roots — `~/.guppy` does not exist.
- A managed project writes only inside its own tree (`.guppy/`,
  `.direnv/`) and Bazel's shared cache.
- `guppy uninstall` removes the binary, the rc-file hook lines it added,
  and — with `--purge` — the data and cache directories.
