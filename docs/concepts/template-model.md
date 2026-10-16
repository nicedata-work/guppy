# Template model

A guppy template is a manifest plus a payload. Nothing else: no template
engine, no plugin runtime, no executable hooks — the renderer is a tiny,
pure function, which is what makes `guppy dev lint` able to statically verify
every substitution and `guppy dev test` able to produce deterministic golden
snapshots.

## Layout

```text
go-cli/
├── guppy.toml            # manifest (the only required file)
├── template/             # payload — rendered verbatim except {{ .Var }} substitution
│   ├── MODULE.bazel
│   ├── .bazelversion
│   ├── .nvmrc
│   ├── .gitignore.hbs    # `.hbs` suffix marks a rendered file
│   └── ...
└── extends: (declared in guppy.toml, not a file)
```

- The payload lives under `template/` so manifests, docs, and test fixtures
  can sit beside it without leaking into scaffolds. A missing `template/`
  dir means the root *is* the payload (thin templates).
- A filename ending in `.hbs` is rendered and the suffix stripped.
  Everything else is copied byte-for-byte — `MODULE.bazel` and `.envrc` stay
  reviewable as-is. Dotfiles are ordinary payload files; `.hbs` on a dotfile
  works (`.gitignore.hbs` → `.gitignore`).

## The manifest: `guppy.toml`

```toml
id          = "go-cli"                 # unique within a scope; [a-z0-9-]
version     = "0.3.1"                  # semver
kind        = "cli"                    # cli | service | lib | app | docs | infra
language    = "go"                     # go | ts | python | rust | none
description = "Go CLI with cobra, golangci-lint, and Bazel"

extends  = ["base-toolchain"]         # declarative inheritance
requires = { guppy = ">=1.2" }        # minimum engine version

[vars]                                # interactive prompts, in this order
module_path = { prompt = "Go module path", default = "github.com/acme/{{ .name }}" }

[defaults]                            # all have --no-* inverses
git        = true
bazel      = true
direnv     = true
linters    = true
tests      = true
agents_md  = true

[gp]                                  # resolved into .guppy/project.toml
build = { command = "bazel build //...", requires = ["bazel"] }
test  = { command = "bazel test //...", requires = ["bazel", "tests"] }
lint  = "bazel run //:gazelle -- -mode=diff"

[includes]                            # narrow-only filters: can subtract
include = ["**/*"]                    # from what the directory contains,
exclude = ["template/scratch/**"]     # never expand it
```

Validation rules are enforced by `guppy dev lint` — errors refuse a scaffold,
warnings do not. Notables: `id`/`version`/`kind`/`language` must be present
and well-formed (**error**); a `[vars]` var referenced in the payload but
not declared is an **error** (the strict allow-list — undeclared `{{ .X }}`
is never silently empty); `[gp]` verbs must be well-formed and may not
shadow the built-in `doctor`.

## Variable substitution

Deliberately small — no logic, no partials, no control flow:

```text
{{ .name }}                 required var, prompted if absent
{{ .module_path }}          declared var with prompt/default
{{ .name | kebab }}         filters: kebab, snake, upper, lower, basename
{{ if .bazel }}…{{ end }}    the ONLY conditional: default toggles, boolean, blocks
```

A payload file whose entire content is one `{{ if .toggle }}…{{ end }}`
block is **toggle-gated**: when the toggle is off the file is not emitted at
all (`--no-agents-md` means no `AGENTS.md`, not an empty one). Gating on a
toggle not declared in `[defaults]` is a lint error even when the file would
be omitted — static verifiability is the whole point.

Anything more complex belongs in the consuming project, not the template.

## The environment contract

This is the single most opinionated thing every first-party template ships:

- **Pins are the source of truth** — `.bazelversion`, `.nvmrc`, the pnpm
  pin. Nothing is resolved from the machine's global state.
- **Bazel is the hermetic provider** — toolchains declared in
  `MODULE.bazel`, fetched into Bazel's shared cache, absent from the global
  environment; builds and pinned-tool invocations run as sandboxed Bazel
  targets.
- **direnv is the activation mechanism** — `.envrc` wires direnv to Bazel:
  on `cd`, pinned tools are built and exec shims land in `.direnv/bin/`
  (never outside the project tree); on `cd`-out they unload. The nix-shell
  feel with zero nix.
- **No global pollution, ever** — a project contributes exactly two things
  to the machine: entries in Bazel's shared cache and files inside its own
  tree.

Templates own this wiring; `guppy doctor` audits it, and because the
contract is uniform, guppy can regenerate or repair it mechanically
instead of guessing per-project conventions.
