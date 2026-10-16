# Guppy: a CLI for scaffolding projects

```text
               ____pppppq888888888qqqqq___,
        ._ppp8888888888888888888888888888888qqq_
     _pp8888888888888888888888888888888888888888q_
    p888888888888888888888888088o%8888888888888888q,      _pq_
   q8888888▗▄▖888888▗▄▖888%#8888#8888888888888888888p  _p88888p
  |888888%8▝▀▘8╰──╯8▝▀▘88888#8088#88o08#808#8#8888888qp88888888
  d88888888o88#888#8888888%88o8888888#888888888888888888888888(
  88888%88888888880888888o88o88o08888%8888%8888o8888888888888P
  8888888888888888888888%88888888%8%888888o888888888888888888'
  )88888888888888888888%8888888888888%88888888888888888888888
   "888888888888888888888#88808#88888888808888888888888888888.
    '"8888888888888888888888888888888888888888888888888888888,
       '"888888888888888888888888888888888888888P"'"888888888|
            ""Y88888888888888888888888888888P"'       "Y8888P
                  ""Y88888888888888888P""'
```

## Summary
A CLI for generating new projects based on templates with great defaults.

## Features
- Quickly scaffold projects enabling getting a dev environment setup in no time
- Language or target system agnostic
- Rich library of templates both first party and community contributed, managed through CLI as plugins
- Built in linting, debugging to support new template development
- `guppy doctor` in a project directory can audit project structure and recommend fixes
- dependency management including tools/packages
- self-installing (e.g. git, direnv, bazelisk)
- maintains catalog and trace of created projects via shellhooks and project metadata files created on scaffolding

## Design
- a central repository of first party templates that are interacted with via cli (list/add/remove/clone)
- a central community repository maintained and likewise reachable via cli with an appending of a `community:` source prefix (e.g. `guppy template add community:acme-widget`)
- great defaults ie. git + contextual .gitignore, bazel(isk), relevant linters + testing by default, bare bones AGENTS.md
- templates are extendable. A new one can extend an existing one

## Tech
- Manages dependencies and using bazel + direnv + git to drive development environment (think nix-shell experience when cd'ing into the project directory)
- Written in Golang

## Building and testing

Everything goes through Bazel (hermetic, pinned via `.bazelversion`); no
system Go is needed to build or test — rules_go fetches the Go SDK itself.

```sh
bazel build //...              # build every target; binary at
                               #   .bazel/bin/cmd/guppy/guppy_/guppy
bazel test  //...              # run all tests
bazel test  //internal/cli:cli_test   # run one package's tests
```

Inside the repo no install step is needed: `.envrc` prepends the build
output dir (`.bazel/bin/cmd/guppy/guppy_`) to PATH ahead of everything
else, so `guppy` and `gp` resolve to the binary `bazel build //...` just
produced (and fall back to the installed guppy when there is no build).

To use the fresh build from shells *outside* the project, copy it to the
installer's location (same directory, `install -D` creates it if
missing):

```sh
install -Dm755 .bazel/bin/cmd/guppy/guppy_/guppy ~/.local/bin/guppy
```

After adding or moving Go packages, regenerate the BUILD files and keep
them committed (CI fails when they are stale):

```sh
bazel run //:gazelle                        # rewrite BUILD files
bazel run //:gazelle -- -mode=diff          # what CI checks (must be empty)
```

Formatting, linting, and docs also need a local Go toolchain (`go` on
PATH — used only by these checks, never by the build):

```sh
gofmt -s -l .                    # must print nothing
go vet ./...                     # plus golangci-lint (CI: lint job)
go fix -diff ./...               # modernizer suggestions are failures
./scripts/gen-docs.sh            # regenerate docs/cli/ (CI: --check)
./scripts/fuzz.sh                # time-boxed fuzz smoke over all targets
```

With direnv hooked up (`direnv allow`), the same commands are available
contextually as project verbs — `gp build`, `gp test`, `gp lint` (from
this repo's own `.guppy/project.toml`).

## Documentation

User-facing docs follow `uv`'s structure:

- [Introduction](docs/index.md) — what guppy is, highlights, quick example
- [Getting started](docs/getting-started/index.md) — installation, first steps, features, help
- [Guides](docs/guides/index.md) — creating projects, using templates, `gp` workflows, authoring, publishing
- [Concepts](docs/concepts/index.md) — template model, scopes & resolution, `extends`, `gp`, doctor & catalog, trust
- [Reference](docs/reference/index.md) — CLI, settings, paths, env vars, troubleshooting

- [CLI reference](docs/cli/guppy.md) — generated from the cobra tree (`guppy docs`)
