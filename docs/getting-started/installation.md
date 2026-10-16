# Installation

## Standalone installer

The canonical path mirrors `uv`: a standalone script piped into a shell, no
package manager required.

Linux and macOS:

```sh
curl -LsSf https://nicedata.work/guppy/install.sh | sh
```

Windows (PowerShell):

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://nicedata.work/guppy/install.ps1 | iex"
```

The nicedata.work URLs are a convenience front for the release assets;
the installer and binaries can also be fetched directly from GitHub
Releases, per release or pinned:

```sh
curl -LsSf https://github.com/nicedata-work/guppy/releases/latest/download/install.sh | sh
```

The installer:

- detects OS/arch (linux/windows × x86_64/aarch64, macOS arm64), downloads the
  checksummed release tarball, verifies it, and unpacks the single static
  binary;
- installs to `~/.local/bin/guppy` (the same on Linux, macOS, and
  `%USERPROFILE%\.local\bin` on Windows), overridable with
  `GUPPY_INSTALL_DIR`;
- prepends the install dir to `PATH` by editing your shell rc file, unless
  `--no-modify-path` / `GUPPY_NO_MODIFY_PATH` is set;
- never runs as root and works unattended (non-TTY → no prompts), so it is
  safe in CI;
- prints next steps (`guppy shell-env`, `guppy new`) on success.

## After installing

Two setup steps make guppy ambient:

```sh
guppy shell-env    # one-line eval in your rc file: keeps the project index fresh
guppy new           # scaffold a project — see First steps
```

## Updating and uninstalling

Updates and removal are CLI verbs, not re-runs of the installer:

```sh
guppy update                 # in-place binary replace, checksum-verified,
                             # previous binary restored on failure
guppy update --all           # the binary and all installed templates
guppy template update        # update installed templates (separate concern)
guppy update .               # move the current project to its template's
                             # latest version
guppy uninstall [--purge]    # remove the binary, rc-file hook lines, and data
```

`guppy update` alone manages the executable only; templates and projects
are the `--all` / `.` forms, so plain "update" never means two things.

## Package managers

Homebrew, apt, and scoop distribution is supported but secondary; the
installer remains the canonical path.

## Where things live

guppy follows the XDG layout — settings in `~/.config/guppy/guppy.toml`,
data in `~/.local/share/guppy/`, cache in `~/.cache/guppy/`, and nothing at
`~/.guppy`. See [Paths and storage](../reference/paths.md) for the full
table.
