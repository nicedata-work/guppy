# guppy

A CLI for scaffolding new projects from templates with great defaults —
language- and target-agnostic. Written in **Go**, built with **Bazel**.
`uv`-style CLI UX; `pi`-style resource management.

## Highlights

* One command to a working project: `guppy new` produces git + contextual
  `.gitignore`, bazel/bazelisk, pinned toolchains, linters + testing, a
  bare-bones `AGENTS.md`, and direnv wiring — in seconds.
* **Hermetic dev environments, zero nix**: Bazel toolchains + direnv give the
  nix-shell feel — `cd` in, get exactly the project's pinned tools; `cd` out,
  nothing leaked. No `npm install -g`, ever.
* Language- and target-agnostic: Go, TypeScript, Python, Rust, or none —
  scaffolds are composed from orthogonal axes (`--kind`, `--lang`, toggles),
  not an exploding catalogue of named templates.
* First-party and community templates managed through the CLI
  (`guppy template list/add/remove/clone/update`), with community templates
  always an explicit, per-template trust decision.
* Templates are **declarative** (`guppy.toml` + payload) and **extendable**
  (`extends`); no template engine, no plugin runtime, no executable hooks.
* First-class authoring tooling: `guppy dev lint` and `guppy dev test` (golden
  snapshot tests) without leaving the CLI.
* Every managed project is traceable: a committed `.guppy/project.toml`
  marker plus a global index kept fresh by a shellhook.
* `cd` into a managed project and the project face of the CLI — `gp` — is on
  PATH contextually: `gp build`, `gp test`, `gp lint`, `gp doctor` from
  minute zero.

## Installation

Install guppy with the official standalone installer:

```sh
curl -LsSf https://nicedata.work/guppy/install.sh | sh
```

Then set up the shellhook and scaffold your first project — see
[first steps](getting-started/first-steps.md).

guppy can also be installed from a package manager where available; the
installer remains the canonical path — see the
[installation page](getting-started/installation.md).

## Projects

`guppy new` scaffolds a project whose dev environment is pinned from the
first commit, and whose workflow verbs are already wired:

```sh
$ guppy new data-handbook --kind app --lang ts
$ cd data-handbook
$ gp build        # ≡ guppy project build — runs the project's Bazel targets
$ gp test
$ gp lint
$ gp doctor       # audit the project against its template
```

`gp` is guppy in **project mode** — one binary, two faces. Invoked as
`guppy` it is the global management CLI (templates, settings, doctor);
inside a guppy-managed project it dispatches the project's own verbs. See
the [project guide](guides/gp-workflows.md).

## Learn more

See the [first steps](getting-started/first-steps.md) or jump straight to
the [guides](guides/index.md). The [concepts](concepts/index.md) section
explains how templates, scopes, and the trust model work; the
[reference](reference/index.md) lists every command, setting, and path.
