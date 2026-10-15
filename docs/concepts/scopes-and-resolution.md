# Scopes, resolution, and identity

## Scopes

A template can arrive from four scopes, layered by trust:

| Scope | Location | Source | Trust |
|---|---|---|---|
| **built-in** | embedded in the binary | shipped with guppy | full |
| **user** | `~/.local/share/guppy/templates/` | user-authored or `template add` | full |
| **local** | any path passed in place (`guppy new ./my-template`) | loaded, never copied | full (you wrote it) |
| **community** | `~/.local/share/guppy/community/<id>@<version>` | `guppy template add community:<source>` | **explicit per-template review before first use** |

Community scope is selected by the `community:` source qualifier — a prefix
on any source spec (`community:acme`,
`community:registry:acme@1.2.0`, `community:git:…@ref`) — rather than a
flag, so the same grammar works for `add`, `remove`, `update`, and `clone`.
It is *not* a namespace that changes resolution: trust is a property of the
scope, not the qualifier, and a community install is usable only once its
review-and-confirm record exists.

## Resolution

`guppy new --template <id>` resolves in scope order **built-in → user →
registry chain**, first match wins. Built-ins always win for a colliding
id, so a malicious or stale community template cannot shadow a
first-party one; the collision prints a warning.

The registry chain is the ordered list of registries guppy consults for
registry ids: named team registries (from `[template.registries]` in
`guppy.toml`) in declaration order, then the default public registry.
First hit wins — an org can shadow a public id deliberately. A
not-found falls through to the next registry; a network failure aborts
(a team registry being down must never silently fall back to the public
one). `registry:<name>:<id>[@<version>]` targets exactly one named
registry; `guppy template search` (no query lists everything) searches
the whole chain and labels which registry each hit came from.

A registry is either an http(s) URL or a **git repo** — the checkout is
the registry (`index.json` at the root, tarballs beside it), cloned with
the user's own git credentials, so a private org repo or Azure DevOps
repository needs no token configuration.

With no `--template`, `guppy new` picks from the **kind × language** matrix
(`kind=cli, language=go` → built-in `go-cli`), failing with a list of
candidates when ambiguous — `--template` is the explicit override.

## Identity

A template's canonical identity depends on its source:

- **registry** → `<registry>/<id>@<version>` (version-pinned)
- **git** → repo URL without ref + the resolved commit SHA (the ref is
  pinned at add time; `guppy template update` reconciles the checkout,
  never moves the ref)
- **local** → resolved absolute path (loaded in place, never copied)

The same identity encountered through two routes is one template. Two
*different* identities claiming the same `id` produce a warning, and the
higher scope wins deterministically — never silent duplication.
