# Guppy: a CLI for scaffolding projects

```text
[39m               [38;2;254;254;254m____pppppq888888888qqqqq___,[0m
[39m        [38;2;254;254;254m._ppp8888888888888888888888888888888qqq_[0m
[39m     [38;2;254;254;254m_pp888888888888888[38;2;84;51;127m888888888888[38;2;254;254;254m8888888888888q_[0m
[39m    [38;2;254;254;254mp88888888[38;2;84;51;127m8888888888888888088o%8888888888[38;2;254;254;254m888888q,[39m      [38;2;254;254;254m_pq_[0m
[39m   [38;2;254;254;254mq8888[38;2;84;51;127m888[38;2;254;254;254m▗▄▖[38;2;84;51;127m888888[38;2;254;254;254m▗▄▖[38;2;84;51;127m888%#8888#88888888888888[38;2;254;254;254m88888p[39m  [38;2;254;254;254m_p88888p[0m
[39m  [38;2;254;254;254m|8888[38;2;84;51;127m88%8[38;2;254;254;254m▝▀▘[38;2;84;51;127m8[38;2;254;254;254m╰──╯[38;2;84;51;127m8[38;2;254;254;254m▝▀▘[38;2;84;51;127m88888#8088#88o08#808#8#88[38;2;254;254;254m88888qp88888888[0m
[39m  [38;2;254;254;254md8888[38;2;84;51;127m8888o88#888#8888888%88o8888888#888888888888[38;2;254;254;254m888888888888([0m
[39m  [38;2;254;254;254m8888[38;2;84;51;127m8%88888888880888888o88o88o08888%8888%8888o8888[38;2;254;254;254m8888[38;2;84;51;127m8[38;2;254;254;254m8888P[0m
[39m  [38;2;254;254;254m88888[38;2;84;51;127m88888888888888888%88888888%8%888888o88888888888888[38;2;254;254;254m8888'[0m
[39m  [38;2;254;254;254m)88888[38;2;84;51;127m888888888888888%8888888888888%8888888888888888888[38;2;254;254;254m8888[0m
[39m   [38;2;254;254;254m"8888888[38;2;84;51;127m88888888888888#88808#888888888088888[38;2;254;254;254m8888888[38;2;84;51;127m888[38;2;254;254;254m8888.[0m
[39m    [38;2;254;254;254m'"8888888888[38;2;84;51;127m888888888888888888888888888[38;2;254;254;254m888888888888888888,[0m
[39m       [38;2;254;254;254m'"8888888888888[38;2;84;51;127m888888888888888[38;2;254;254;254m88888888888P"'"888888888|[0m
[39m            [38;2;254;254;254m""Y88888888888888888888888888888P"'[39m       [38;2;254;254;254m"Y8888P[0m
[39m                  [38;2;254;254;254m""Y88888888888888888P""'[0m
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

## Documentation

User-facing docs follow `uv`'s structure:

- [Introduction](docs/index.md) — what guppy is, highlights, quick example
- [Getting started](docs/getting-started/index.md) — installation, first steps, features, help
- [Guides](docs/guides/index.md) — creating projects, using templates, `gp` workflows, authoring, publishing
- [Concepts](docs/concepts/index.md) — template model, scopes & resolution, `extends`, `gp`, doctor & catalog, trust
- [Reference](docs/reference/index.md) — CLI, settings, paths, env vars, troubleshooting

- [CLI reference](docs/cli/guppy.md) — generated from the cobra tree (`guppy docs`)
