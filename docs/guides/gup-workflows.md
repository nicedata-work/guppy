# Working on projects

Inside a guppy-managed project — any directory with `.guppy/project.toml`
(or a descendant of one) — the project face of the CLI runs the project's
own workflow verbs.

## The baseline verbs

Every first-party template ships the baseline set, so a freshly scaffolded
project has its workflow from minute zero:

```sh
gup build      # ≡ guppy project build — run the project's build targets
gup test       # run the project's tests
gup lint       # run the project's linters
gup doctor     # audit this project (guppy doctor, project scope)
```

Verbs execute through the pinned environment (the direnv/Bazel toolchain),
so `gup test` uses exactly this repo's pinned node/pnpm/bazel — never
whatever happens to be global.

## Where the verbs come from

The template manifest declares `[gup]` entries; the scaffold resolves them
into `.guppy/project.toml`:

```toml
[gup]
build = "bazel build //..."
test  = "bazel test //..."
lint  = "bazel run //tools/env:lint"   # template-chosen
# doctor is built in — never overridden
```

Entries are either a bare command string or
`{ command = "…", requires = ["bazel", "linters"] }`: verbs whose required
axes are off at scaffold time are dropped from the written map. Commands may
reference `[vars]` (`{{ .module_path }}`) with the same strict allow-list as
the payload.

Templates can add verbs beyond the baseline four (`gup dev`, `gup deploy`);
an unknown verb prints the map.

## `gup` is ambient

The direnv setup script links `gup` into `.direnv/bin/` alongside the tool
shims: `cd` into the project and `gup test` works; `cd` out and `gup` is
gone. If direnv is not hooked up, the canonical form works from anywhere:

```sh
guppy project test        # walks up to the nearest .guppy/project.toml
```

Outside a managed project, project mode refuses to run — `guppy doctor`
flags the missing direnv hook.

## Auditing the project

`gup doctor` (or `guppy doctor`) is read-only and manifest-driven: it diffs
the project tree against the template's expected file set — missing,
renamed, or deleted payload files; stale pins like `.bazelversion` versus
the template's current default — and emits recommendations.
`--fix` applies the safe subset: restoring missing pinned files never
touches user-edited content. See
[Doctor and the catalog](../concepts/doctor-and-catalog.md).

## Upgrading a project's template

```sh
guppy template update <id>   # bring the installed template forward
guppy doctor                 # see exactly which scaffold files a newer
                             # template version would change
```

The marker file records the template identity and version the project was
scaffolded from, which is what makes this diff precise.
