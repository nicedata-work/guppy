# AGENTS.md — Guppy

Guppy (`guppy`) is a CLI for scaffolding new projects from templates with
great defaults — language- and target-agnostic. Written in **Go**, built
with **Bazel**.

## Commands

All development goes through Bazel (hermetic, pinned via `.bazelversion`
and `.nvmrc`). With direnv hooked up (`direnv allow`), `gup build`, `gup
test`, and `gup lint` run the same commands contextually.

```sh
bazel build //...                        # build everything
bazel test //...                         # run all tests
bazel run //:gazelle                     # regenerate BUILD files after
                                         # adding/moving packages
gofmt -s -l .                            # formatting (CI requires zero output)
```

CI additionally runs golangci-lint (vet, gofmt -s, modernizers) and style
gates — see [.github/workflows/ci.yml](.github/workflows/ci.yml). Keep
`gazelle` output clean: CI fails if BUILD files are stale.

## Structure

```
cmd/guppy/            entry point (cobra command tree)
internal/             all implementation packages:
  cli/                command implementations, output/exit-code policy
  manifest/           guppy.toml parse + validation
  render/             the pure substitution engine; produces FileSet
  templates/          template identity, sources, scope resolution
  scaffold/           file materialization (never-clobber, rollback)
  registry/ dist/     registry client, source pinning, archives
  project/ catalog/   project mode (gup), the global project index
  doctor/ settings/   project audit, user settings
  selfmanage/         guppy update / uninstall
  archive/ fileutil/ httpx/ paths/ diffutil/ version/   support packages
docs/                 documentation (user guides under getting-started/,
                      guides/, concepts/, reference/; generated CLI
                      reference in docs/cli/)
install/              curl-to-shell installer scripts
scripts/              release, fuzz, and smoke-test helpers
tools/                build-time utilities (fuzzrank)
.guppy/               guppy's own project marker (dogfooding)
```

## Conventions

- Generated files are never hand-edited: `docs/cli/` comes from
  `guppy docs` (CI checks staleness); BUILD files come from gazelle.
- Templates are declarative data (`guppy.toml` + payload): no executable
  hooks, no plugin runtime, ever.
- Comments explain invariants and non-obvious constraints, not what the
  code obviously does.
