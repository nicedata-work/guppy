# Using templates

Templates come from four scopes — built-in (inside the binary), user
(`~/.local/share/guppy/templates/`), local (any path loaded in place), and
community (installed by explicit decision). See
[Scopes, resolution, and identity](../concepts/scopes-and-resolution.md) for
the mechanics.

## Browsing

```sh
guppy template list                # installed: id, version, kind, lang, scope, source
guppy template list --all-scopes
guppy template search web          # registry index; community repos advertise
                                   # the guppy-template keyword
guppy template show go-cli         # the fully merged payload view + extends
                                   # chain + vars — what would actually be written
```

## Installing

`guppy template add` accepts three source forms:

```sh
guppy template add go-cli                          # built-in id (no-op) or registry id
guppy template add registry:acme-widget@1.2.0      # version-pinned
guppy template add git:github.com/acme/tpl@v2      # ref pinned at add time
guppy template add ./my-template                   # local path, used in place
```

## Community templates and the trust gate

Community sources are selected with the `community:` qualifier — a prefix on
any source spec — rather than a flag, so the same grammar works for `add`,
`remove`, `update`, and `clone`:

```sh
guppy template add community:acme-widget
guppy template add community:git:github.com/acme/tpl@v2
```

Community installs are **not** a default trust decision. `guppy template
add community:…` prints the manifest, the file list, and the diff against a
previously-installed version, and requires an interactive review-and-confirm
before the template is usable. Non-interactive contexts pass `--yes`
explicitly. Community *updates* re-enter the gate — a new version is a new
trust decision. First-party templates never require confirmation.

## Private team registries

Teams keep their own template catalogs — a data engineer maintaining
pipeline templates for their team, for example. Guppy supports this as a
first-class registry chain: named registries in your global
`~/.config/guppy/guppy.toml`, searched in declaration order *before* the
default public registry.

### The paved path: scaffold the registry repo

Don't build the plumbing by hand — the built-in `template-registry`
scaffolds the whole thing:

```sh
guppy new --template template-registry pipeline-templates
cd pipeline-templates
# push to your org (a private GitHub repo, an Azure DevOps git repo — any git remote)
```

What lands in the repo:

- `templates/` — the drop zone: one directory per team template
- `scripts/build-registry.py` — rebuilds `index.json` + `tarballs/`
  from `templates/` (deterministic; `--check` fails on drift)
- `scripts/validate-templates.sh` — `guppy lint` + `guppy test` on every template
- `.github/workflows/registry.yml` — CI: validate on PRs; rebuild and
  commit the registry on merge, so the repo's checkout is always a
  complete, current registry

Team members drop a template into `templates/<id>/` and open a PR; CI
validates it; merge publishes it (a new `index.json` entry + tarball).
Consumers configure guppy once — see below — and `guppy template
search` / `guppy new --template <id>` / `guppy template update` all
resolve from the team registry.

### Pointing guppy at the team registry

The team admin configures it once — and because the registry is just a
git repository, it rides the credentials you already have (SSH keys,
credential managers): **no tokens**.

```toml
# ~/.config/guppy/guppy.toml
[[template.registries]]
name = "team"
git = "git@ssh.dev.azure.com:v3/acme/pipeline-templates"   # Azure DevOps
# git = "github.com/acme/pipeline-templates"               # or GitHub
# ref = "main"                                             # default: HEAD
# trust = "full"   # optional: skip the review gate for org-controlled registries
```

The registry repo's checkout *is* the registry: `index.json` at the root,
tarballs beside it (`tarballs/…`, referenced by repo-relative paths) —
the same layout as the [public
registry](https://github.com/nicedata-work/guppy-registry), so a team can
fork that repo's CI and use it as a starting point.

Then everything just works:

```sh
guppy template search                    # lists team templates (labeled "team") and public ones
guppy new --template dbt-pipeline        # resolves from the team registry
# or pinned and explicit:
guppy template add registry:team:dbt-pipeline@1.4.0 --yes
guppy template update team:dbt-pipeline
```

Rules of the chain:

- **First hit wins.** A team registry can shadow a public id
deliberately; `registry:<name>:<id>` disambiguates explicitly.
- **A not-found falls through; a network failure aborts.** If the team
registry is unreachable, guppy stops with a network error rather than
silently scaffolding from the public registry.
- **Git registries need no tokens**: cloning uses your existing git
credentials (SSH keys, credential managers), so private GitHub org
repos and Azure DevOps repositories just work. HTTP-served private
indexes instead ride `GUPPY_REGISTRY_<NAME>_TOKEN` (dashes become
underscores), sent as `Authorization: Bearer` on that registry's
requests. Per-template git access (`community:git:…@ref`) needs no
token either, for the same reason.
- **Trust stays explicit.** By default installs from any remote registry
pass the review-and-confirm gate (once per version). Registries your
org controls and vouches for can set `trust = "full"` to skip it — the
gate is org policy, not per-user ceremony.

## Updating and removing

```sh
guppy template update acme-widget          # one template
guppy template update community:acme --all # by scope, or everything
guppy template remove community:acme-widget
```

`guppy template update` reconciles installed checkouts (git sources are
re-pinned to the resolved SHA); it never moves a ref silently. Updating
templates is separate from updating guppy itself (`guppy update`) — "update"
never means two things.

## Cloning for editing

```sh
guppy template clone community:acme-widget --into ~/src/my-templates
```

Writes the payload out as a local template you can edit; see
[Authoring templates](authoring-templates.md). A local template loaded from
its own path is always full-trust — you wrote it.
