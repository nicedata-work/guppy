# Creating projects

`guppy new` produces a working, opinionated project in seconds. This guide
covers the flags; for what lands on disk, see
[First steps](../getting-started/first-steps.md).

## Choosing a template: axes over names

Rather than an exploding catalogue of named templates, `guppy new` composes
orthogonal axes, `uv init`-style:

```sh
guppy new myservice --kind service --lang go
```

- `--kind K` — `cli | service | lib | app | docs | infra`
- `--lang L` — `go | ts | python | rust | none`
- `--template <id>` — explicit override; otherwise the built-in template
  matching the axes is used

With no `--kind`/`--lang`/`--template`, guppy fails with a list of
candidates when the choice is ambiguous — the flags imply the template, and
`--template` is the escape hatch.

`--bare` scaffolds the minimal tree. `--dry-run` prints what would be
written without touching the filesystem.

## Every default has an inverse

Defaults are opinions; the escape hatch is always mechanical and
discoverable:

```sh
guppy new tool --no-git --no-bazel --no-direnv --no-linters --no-tests --no-agents-md
```

Each `--no-*` flag turns off one `[defaults]` axis of the template. Verbs
whose required axes are off are dropped from the project's command map
(`--no-bazel` yields no `bazel build` verb).

## Variables

Templates prompt for `[vars]` in declaration order. Answer them
non-interactively with `--var`:

```sh
guppy new widget --var module_path=example.com/acme/widget
```

Vars may themselves reference other vars (`default =
"github.com/acme/{{ .name }}"`); they resolve to a fixed point,
order-independent. Missing vars in non-interactive contexts are an error
(exit 2), never silently empty.

## Scaffolding into an existing directory

`guppy new .` (or any non-empty directory) is refused (exit 2) unless
`--force` is passed. Even with `--force`, merge **never clobbers**:
differing files are kept and reported, and only missing template files are
added. This existing-tree awareness is what powers repair-style flows.

## Global flags

Every subcommand takes the same global flags: `-q/-v`,
`--color auto|always|never`, `--no-progress`, `--offline`, `--directory`,
`--config`. Every option also has a `GUPPY_*` environment equivalent — see
[Environment variables](../reference/environment.md).

Exit codes: `0` ok · `1` lint/render errors · `2` unresolved input · `3`
network/registry failure · `130` interrupted (partial work rolled back).
