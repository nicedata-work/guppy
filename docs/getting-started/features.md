# Features

A tour of what guppy does. Everything here is explained in depth under
[concepts](../concepts/index.md).

## Scaffolding with great defaults

`guppy new` composes orthogonal axes — kind (`cli|service|lib|app|docs|infra`)
× language (`go|ts|python|rust|none`) × toggles — over a curated set of
templates, `uv init`-style. Every default has a mechanical, discoverable
inverse (`--no-bazel`, `--no-git`, …). Pins (`.bazelversion`, `.nvmrc`, the
pnpm pin) are part of the scaffold, so a fresh project is reproducible
before its first commit.

## Hermetic environments (Bazel + direnv)

The nix-shell outcome with zero nix: tool versions are pinned in files,
Bazel fetches toolchains into its shared cache, and direnv exposes them on
`PATH` via `.direnv/bin/` shims on `cd` — and unloads on `cd`-out. A project
contributes exactly two things to your machine: entries in Bazel's cache and
files in its own tree. See the
[environment contract](../concepts/template-model.md#the-environment-contract).

## One binary, two faces

`guppy` manages templates and configuration globally; `gp` (`guppy project`)
runs a project's own verbs — `build`, `test`, `lint`, `doctor` — dispatched
through the project's `.guppy/project.toml` command map, executed through
the project's pinned environment. Templates can add verbs beyond the
baseline four (`gp dev`, `gp deploy`).

## Declarative, extendable templates

A template is a `guppy.toml` manifest plus a payload directory. Substitution
is a small, statically verifiable allow-list — no template engine, no
executable hooks, no plugin runtime, ever. Templates inherit declaratively
via `extends`, and `guppy template show` renders the fully merged view so
inheritance is never invisible.

## First-party and community templates, clearly separated

Built-in templates ship inside the binary and work offline. Community
templates install through the same CLI commands with a `community:` source
qualifier — and every community install and update passes an explicit
review-and-confirm gate before first use. See
[trust and security](../concepts/trust-and-security.md).

## First-class authoring

The authoring loop is `edit → guppy dev lint && guppy dev test` with no reinstall
step: templates are ordinary directories loaded in place. `guppy dev test`
renders golden snapshots from a declared var matrix; `guppy dev lint` statically
verifies every substitution in the payload resolves.

## Traceability

Every scaffold writes `.guppy/project.toml` (committed, deterministic, no
secrets), and a shellhook-fed global index (`guppy project list`) tracks managed
projects on the machine. `guppy doctor` diffs a project against its
template — missing files, stale pins — and `--fix` restores the safe subset.
