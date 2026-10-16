# First steps

This walkthrough takes you from installed to a working, tested project in
under a minute.

## 1. Hook up your shell

```sh
guppy shell-env
```

This emits a shellhook (zsh/bash) — a one-line `eval` in your rc file. The
hook keeps the global project index fresh on `cd` and makes `gp` available
contextually inside managed projects. The hook is pure shell: it never
invokes guppy on the hot path, and if it fails your shell is unaffected —
the index just goes stale until the next fold.

## 2. Scaffold a project

```sh
$ guppy new data-handbook --kind app --lang ts
```

With no flags, `guppy new` picks from the **kind × language** matrix
(`--kind cli|service|lib|app|docs|infra`, `--lang go|ts|python|rust|none`),
failing with a list of candidates when ambiguous. Every default has a
`--no-*` inverse (`--no-bazel`, `--no-linters`, `--no-agents-md`, …), and
`--var key=value` answers template variables non-interactively.

What lands in the directory:

- git + a contextual `.gitignore`
- `MODULE.bazel`, `.bazelversion`, `.nvmrc` — pinned toolchains, reproducible
  before the first commit
- linters + testing wired as Bazel targets
- a bare-bones `AGENTS.md`
- `.envrc` + `tools/env/` — the direnv wiring that makes the environment
  hermetic (see the [environment contract](../concepts/template-model.md#the-environment-contract))
- `.guppy/project.toml` — the project marker recording template identity,
  vars, toggles, and the `[gp]` command map

## 3. Work on the project

```sh
$ cd data-handbook
$ gp build          # the project's build targets, via its pinned Bazel
$ gp test
$ gp lint
$ gp doctor         # audit against the template; --fix applies the safe subset
```

`gp` is `guppy project` shimmed into `.direnv/bin/` — `cd` in and it is on
PATH; `cd` out and it is gone, exactly like the nix-shell experience. If
direnv is not hooked up, `guppy project <verb>` works from anywhere.

## 4. Look around the template universe

```sh
guppy template list             # what is installed, by scope
guppy template show go-cli      # the fully merged view of a template
guppy template search web       # search the registry index
```

## Next steps

- [Creating projects](../guides/creating-projects.md) — `guppy new` flags in depth
- [Using templates](../guides/using-templates.md) — sources, community templates, trust
- [Working on projects](../guides/gp-workflows.md) — the `gp` verb set
- [Authoring templates](../guides/authoring-templates.md) — write your own
