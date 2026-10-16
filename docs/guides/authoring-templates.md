# Template authoring guide

How to write, test, and ship a Guppy template. The CLI reference is
generated in [`../cli/`](../cli/guppy.md).

## What a template is

A template is a plain directory — no registration, no build step:

```text
go-cli/
├── guppy.toml            # the manifest (the only required file)
├── tests.toml            # optional: snapshot test matrix for `guppy dev test`
├── testdata/snapshot/    # golden snapshots (`guppy dev test -u` writes these)
└── template/             # payload — written verbatim except {{ }} substitution
    ├── MODULE.bazel
    ├── .bazelversion
    ├── .gitignore.hbs    # `.hbs` marks a rendered file; suffix is stripped
    └── ...
```

A missing `template/` directory means the template root *is* the payload
(thin templates). Dotfiles are ordinary payload files. Everything that is
not under the payload (`guppy.toml`, `tests.toml`, `testdata/`) never
leaks into scaffolds.

During development you just point guppy at the directory — templates are
loaded in place, never copied:

```sh
guppy dev lint ./my-template && guppy dev test ./my-template
guppy new --template ./my-template demo
```

`guppy template clone <id> --into ./my-template` copies an installed
template out for editing.

## The manifest: `guppy.toml`

```toml
id          = "go-cli"                 # required; unique in scope, [a-z0-9-]
version     = "0.3.1"                  # required; semver
kind        = "cli"                    # required; cli | service | lib | app | docs | infra
language    = "go"                     # required; go | ts | python | rust | none
description = "Go CLI with cobra, golangci-lint, and Bazel"

extends  = ["base-toolchain"]         # ordered inheritance; later wins
requires = { guppy = ">=0.2" }        # minimum engine version

[vars]                                # interactive prompts, prompted in order
module_path = { prompt = "Go module path", default = "github.com/acme/{{ .name }}" }

[defaults]                            # toggles; every one gets a --no-* inverse
git        = true
bazel      = true
direnv     = true
linters    = true
tests      = true
agents_md  = true

[gup]                                  # verbs resolved into .guppy/project.toml
build = { command = "bazel build //...", requires = ["bazel"] }
test  = { command = "bazel test //...", requires = ["bazel", "tests"] }
lint  = "bazel run //:gazelle -- -mode=diff"   # bare string = no requirements

[includes]                            # narrow-only filters (never expand)
include = ["**/*"]
exclude = ["template/scratch/**"]
```

Validation (`guppy dev lint`) splits errors from warnings:

| Check | Level |
|---|---|
| `id`/`version`/`kind`/`language` present and well-formed | error |
| `extends` unresolvable in the scope chain | error |
| payload references an undeclared var | error |
| payload gates on an undeclared toggle | error |
| `[gup]` verb name malformed / reserved (`doctor`) / empty command / unknown `requires` axis | error |
| declared var never used | warning |
| missing baseline verb (build/test; + lint for language templates) | warning |
| shadow-crossing a trust boundary in `extends` | warning |

Warnings-not-failures is the pi rule: `guppy new` refuses errors, proceeds
past warnings.

## Variables and substitution

The renderer is deliberately tiny — no logic, no loops, no control flow:

```text
{{ .name }}                 required var (prompted if no default)
{{ .module_path }}          declared var with prompt/default
{{ .name | kebab }}         filters: kebab, snake, upper, lower, basename
{{ if .bazel }}…{{ end }}    the ONLY conditional: boolean toggles, nestable
```

Rules that make `guppy dev lint` able to statically verify every substitution:

- **Strict allow-list.** An undeclared `{{ .X }}` is a lint error and a
  render error — never silently empty.
- **Var defaults may reference other vars** (`default =
  "github.com/acme/{{ .name }}"`). Resolution is order-independent (fixed
  point); cycles are lint errors. `{{ if }}` is not allowed in a default.
- **Toggle-gated files.** A payload file whose whole content is a single
  `{{ if .toggle }}…{{ end }}` block is omitted entirely when the toggle
  is off — `--no-agents-md` means no `AGENTS.md`, not an empty one. The
  toggle must be declared in `[defaults]` even though the file would be
  omitted: static verifiability is the point.

## Inheritance (`extends`)

`extends` is ordered declarative inheritance — never executable hooks.
Each parent contributes payload files, `[defaults]`, and `[vars]`; the
child overlays them file-by-file (child wins whole-file; later `extends`
entries win over earlier ones). Children can override a parent's defaults
and re-prompt its vars but can only narrow, never expand. Chains resolve
depth-first with cycle detection (lint error). `guppy template show <id>`
prints the merged view, so inheritance is never invisible.

## The project contract: `[gup]`

The verbs you declare become `.guppy/project.toml`'s `[gup]` map, and with
it the project face of the CLI (`gup build`, `gup test`, `gup lint`,
`gup doctor` — doctor is built in and reserved). Entries whose `requires`
axes are off at scaffold time are dropped (`--no-bazel` yields no
`bazel build` verb). Commands may use the same strict var allow-list
(`{{ .module_path }}`); an undeclared var is a scaffold error.

First-party templates also ship the environment contract payload
(`MODULE.bazel`, `.bazelversion`, `.bazelrc`, `.envrc` + `tools/env/`,
contextual `.gitignore`) via `base-toolchain`; extend it rather than
reinventing it.

## Snapshot testing (`guppy dev test`)

`tests.toml` declares a var/toggle matrix beside the manifest. The
`default` case (var defaults + default toggles) is implicit:

```toml
[[case]]
name = "minimal"            # required, [a-z0-9-], unique
[case.vars]                  # variable expressions; may reference other vars
module_path = "example.com/x/{{ .name }}"
[case.toggles]               # override default axes for this case
agents_md = false
```

Each case renders to a golden snapshot in
`testdata/snapshot/<case>.toml` — the whole FileSet (path, mode, content)
in one human-diffable TOML file (one data file per case keeps build
tooling out of snapshot dirs). The loop is:

```sh
guppy dev lint ./my-template && guppy dev test ./my-template
# after an intentional change:
guppy dev test -u ./my-template
```

Vars with no value render as `example-<var>` so snapshots stay
deterministic. `guppy dev test` also warns when a `[defaults]` toggle is not
exercised by any case.

## Shipping

- **User scope**: `guppy template add ./my-template` installs it under
  `~/.local/share/guppy/templates/` (full trust — you authored it).
- **Community scope**: publish to a registry (see
  [`publishing-templates.md`](publishing-templates.md)); consumers install
  with `guppy template add community:<source>` and pass an explicit
  per-template review-and-confirm gate before first use. Updates re-enter
  the gate.
- Keep `guppy dev lint && guppy dev test` green in CI; they work offline against
  the directory alone.
