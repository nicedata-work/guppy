<div align="center">

<img src="docs/assets/logo.svg" alt="guppy" width="420">

[![CI](https://github.com/nicedata-work/guppy/actions/workflows/ci.yml/badge.svg)](https://github.com/nicedata-work/guppy/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/nicedata-work/guppy)](https://github.com/nicedata-work/guppy/releases)
[![License](https://img.shields.io/github/license/nicedata-work/guppy)](LICENSE)

**[Installation](#installation) • [Quick start](#quick-start) • [Documentation](docs/index.md)**

</div>

## About

guppy is a CLI for scaffolding new projects from templates with great defaults.
One command produces a working, tested project — git with a contextual
`.gitignore`, pinned toolchains, Bazel, linters, tests, a bare-bones
`AGENTS.md`, and direnv wiring — in seconds, and a single marker file keeps
every created project traceable afterwards.

It is language- and target-agnostic: scaffolds are composed from orthogonal
axes (`--kind` × `--lang` × toggles), `uv init`-style, instead of an
exploding catalogue of named templates. A template is plain data — a
`guppy.toml` manifest plus a payload — with a small, statically verifiable
substitution engine and no runtime of its own.

You may know [cookiecutter](https://cookiecutter.readthedocs.io/) or
[copier](https://copier.readthedocs.io/). guppy goes one step further: every
scaffold gets a hermetic Bazel + direnv environment (the nix-shell
experience, zero nix), a project face of the CLI (`gup`) that appears on `cd`
and disappears on `cd` out, and built-in lint/test tooling for the templates
themselves.

## Features

- One command to a working project: `guppy new` scaffolds git, pinned
  toolchains, Bazel, linters, testing, `AGENTS.md`, and direnv wiring in
  seconds.
- Hermetic dev environments, zero nix: `cd` in and the project's pinned
  tools are on `PATH`; `cd` out and nothing leaks — no `npm install -g`, ever.
- Language- and target-agnostic: Go, TypeScript, Python, Rust, or none —
  orthogonal axes, not an exploding catalogue of named templates.
- Declarative, extendable templates: `guppy.toml` + payload, inheritance via
  `extends`, no template engine, no plugin runtime, no executable hooks — ever.
- First-party and community templates managed through the CLI
  (`guppy template list/add/remove/clone/update`); every community install
  passes an explicit review-and-confirm gate.
- One binary, two faces: `guppy` manages templates and configuration
  globally, `gup` runs a project's own verbs — `build`, `test`, `lint`,
  `doctor` — through the project's pinned environment.
- First-class authoring: `guppy dev lint` and `guppy dev test` (golden
  snapshot tests) with no reinstall step — templates load in place.
- Every scaffold is traceable: a committed `.guppy/project.toml` marker plus
  a shellhook-fed global index, with `guppy doctor` auditing a project
  against its template and `--fix` restoring the safe subset.

## Installation

<details>
<summary>Standalone installer</summary>

The canonical path mirrors `uv`: a standalone script piped into a shell, no
package manager required.

On Linux and macOS:

```bash
curl -LsSf https://nicedata.work/guppy/install.sh | sh
```

On Windows:

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://nicedata.work/guppy/install.ps1 | iex"
```

The installer detects OS/arch, verifies the checksummed release tarball, and
unpacks the single static binary into `~/.local/bin` (`%USERPROFILE%\.local\bin`
on Windows), prepending it to `PATH`. It never runs as root and works
unattended, so it is safe in CI.

</details>

<details>
<summary>GitHub Releases</summary>

Pre-built binaries, the installer, and detached checksums are available from
the [GitHub releases](https://github.com/nicedata-work/guppy/releases) page,
pinnable via `GUPPY_VERSION`.

</details>

<details>
<summary>Package managers</summary>

Homebrew, apt, and scoop distribution are supported but secondary; the
standalone installer remains the canonical path.

</details>

Two setup steps make guppy ambient:

```bash
guppy shell-env    # one-line eval in your rc file: keeps the project index fresh
guppy new           # scaffold your first project
```

Updates and removal are CLI verbs, not re-runs of the installer:

```bash
guppy update                 # in-place, checksum-verified self-update
guppy update --all           # the binary and all installed templates
guppy update .               # move the current project to its template's latest version
guppy uninstall [--purge]    # remove the binary, rc-file hooks, and data
```

## Quick start

- **New to guppy:** follow [First steps](docs/getting-started/first-steps.md) — from
  installed to a built, tested project in under a minute.
- **Scaffolding a project:** see [Creating projects](docs/guides/creating-projects.md) —
  axes, toggles, variables, merging into existing trees.
- **Writing a template:** see [Authoring templates](docs/guides/authoring-templates.md) —
  write, lint, test, and ship without leaving the CLI.

```bash
guppy new data-handbook --kind app --lang ts
cd data-handbook
gup build          # the project's build targets, via its pinned Bazel
gup test
gup lint
gup doctor         # audit against the template; --fix applies the safe subset
```

## Why guppy?

### It is hermetic without nix

- Tool versions are pinned in files (`.bazelversion`, `.nvmrc`), Bazel
  fetches toolchains into its shared cache, and direnv exposes them on `PATH`
  via `.direnv/bin/` shims on `cd` — and unloads them on `cd` out.
- A project contributes exactly two things to your machine: entries in
  Bazel's cache and files in its own tree. See the
  [environment contract](docs/concepts/template-model.md#the-environment-contract).
- A fresh project is reproducible before its first commit: the pins are part
  of the scaffold, not a setup step afterwards.

### Opinions with mechanical escape hatches

- Every default is a flag: `--no-git`, `--no-bazel`, `--no-linters`,
  `--no-agents-md` — turning an axis off drops its verbs from the project's
  command map.
- `--dry-run` prints what would be written without touching the filesystem;
  `guppy new . --force` merges into an existing tree and never clobbers.
- `--var key=value` answers template variables non-interactively; missing
  vars fail loudly (exit 2), never silently empty.

### Templates you can trust

- Substitution is a small, statically verifiable allow-list: no template
  engine, no plugin runtime, no executable hooks — a malicious template has
  nothing to execute. See [trust and security](docs/concepts/trust-and-security.md).
- Community templates always require an explicit, per-template
  review-and-confirm decision before first use.
- `extends` is declarative inheritance, and `guppy template show` renders
  the fully merged view, so inheritance is never invisible.

### Projects you can always trace and repair

- Every scaffold writes a committed, deterministic `.guppy/project.toml`
  marker (template identity, vars, toggles, the `[gup]` command map) — no
  secrets, ever.
- A shellhook-fed global index tracks every managed project on the machine
  (`guppy project list`).
- `guppy doctor` diffs a project against its template — missing files, stale
  pins — and `guppy update .` moves it forward to the latest template
  version.

## Acknowledgements

guppy's CLI UX and documentation structure follow
[uv](https://github.com/astral-sh/uv); the environment model stands on the
shoulders of [Bazel](https://bazel.build/), [direnv](https://direnv.net/),
and the [nix](https://nixos.org/) idea of pinned, reproducible toolchains;
and the scaffolding itself walks a well-worn path first cleared by
[cookiecutter](https://cookiecutter.readthedocs.io/) and
[copier](https://copier.readthedocs.io/).

## Development

Everything goes through Bazel (hermetic, pinned via `.bazelversion`); no
system Go is needed:

```sh
bazel build //...                        # build every target
bazel test  //...                         # run all tests
bazel run //:gazelle                      # regenerate BUILD files after adding packages
gofmt -s -l .                             # formatting (CI requires zero output)
```

See [AGENTS.md](AGENTS.md) for the full command set and
[docs/](docs/index.md) for contributor and user documentation.