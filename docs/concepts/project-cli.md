# The project CLI (`gup`)

guppy is one binary with two faces. Invoked as `guppy`, it is the global
management CLI (templates, settings, `guppy project list`, `guppy update`).
Invoked as `guppy project <verb>` — shimmed under the short name `gup` — it
operates on *this* project.

## Project mode

`guppy project <verb>` walks up to the nearest `.guppy/project.toml`,
refuses to run outside one, and dispatches verbs according to that file's
`[gup]` command map. The canonical form works from anywhere; `gup` is what
makes it ambient.

The marker file is written at scaffold time and is the definition of a
"guppy-managed project": its presence in a directory is the only thing that
makes one. See [Doctor and the catalog](doctor-and-catalog.md).

## The command map

The template manifest declares verbs; the scaffold resolves them into the
marker:

```toml
[gup]
build = "bazel build //..."
test  = "bazel test //..."
lint  = "bazel run //tools/env:lint"
# doctor is built in — never overridden
```

Rules:

- entries are a bare command string or
  `{ command, requires = ["bazel", "linters"] }`;
- verbs whose required `[defaults]` axes are off at scaffold time are
  dropped from the written map (`--no-bazel` yields no `bazel build` verb);
- commands may reference `[vars]` (`{{ .module_path }}`), resolved with the
  same strict allow-list as the payload — an undeclared var is a scaffold
  error, never an empty string;
- `doctor` is always built in and cannot be overridden;
- templates may add verbs beyond the baseline (`gup dev`, `gup deploy`); an
  unknown verb prints the map.

## Executed through the pinned environment

Verbs execute through the project's environment — the direnv/Bazel
toolchain — so `gup test` uses exactly the repo's pinned node/pnpm/bazel,
never global state. This is the environment contract applied to workflow,
not just tooling.

## Contextual availability

The direnv setup script links `gup` into `.direnv/bin/` alongside the tool
shims. `cd` into the project → `gup test` works; `cd` out → `gup` is gone.
If direnv is not hooked up, `guppy project <verb>` is the explicit
fallback, and `guppy doctor` flags the missing hook.
