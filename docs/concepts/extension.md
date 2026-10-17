# Extension (`extends`)

Templates are extendable — as **declarative inheritance**, never executable
hooks.

## Semantics

```toml
extends = ["base-toolchain"]   # ordered list; later entries win over earlier
```

- Each parent's payload is overlaid onto the child's, file-level: the child
  wins whole-file, and later `extends` entries win over earlier ones.
- Parents contribute payload files, `[defaults]`, and `[vars]`. Children can
  override defaults and re-prompt vars, but cannot remove a parent's payload
  file except by overwriting it — narrowing, never expanding (the same rule
  as `[includes]` filters).
- Chains resolve depth-first with cycle detection (**lint error**).

## Inheritance is never invisible

`guppy template show <id>` renders the fully merged view — what will
actually be written — plus the chain that produced it. Nothing about the
merge is implicit at scaffold time.

## First-party family

The first-party family starts at `base-toolchain` (git, `.gitignore`,
bazelisk pins, direnv wiring, `AGENTS.md`), with children like `go-cli` and
`ts-web` (the `MODULE.bazel` + `.nvmrc` + pnpm pin + `.envrc`/`tools/env/`
monorepo shape).

## Why not hooks?

A plugin runtime or executable hook would reintroduce exactly the complexity
the model exists to avoid: non-reviewable scaffolds, non-verifiable
substitutions, and arbitrary code execution at scaffold time. If a template
needs behavior, it ships it as a Bazel target the consuming project invokes
through its `[gup]` command map — visible, pinned, and sandboxed.
