# Registry publishing guide

How to publish templates to a Guppy registry, and what consumers see.

## What a registry is

A registry is a **plain directory of static files** — any static file
server (S3, GitHub Pages, nginx, `python3 -m http.server`) can host one.
No daemon, no database, no upload API:

```text
<registry base URL>/
├── index.json                                  # the index (schema 1)
├── guppy-my-template-1.2.0.tar.gz             # one tarball per version
├── guppy-my-template-1.2.0.tar.gz.sha256      # (recommended; the index checksum is authoritative)
└── guppy-my-template-1.3.0.tar.gz
```

Consumers point guppy at it once, in `~/.config/guppy/guppy.toml`:

```toml
[template]
registry = "https://registry.example.com/guppy"
```

## The index: `index.json`

```json
{
  "schema": 1,
  "templates": [
    {
      "id": "my-template",
      "version": "1.2.0",
      "kind": "cli",
      "language": "go",
      "description": "My first template",
      "keywords": ["guppy-template", "cli", "starter"],
      "tarball": "guppy-my-template-1.2.0.tar.gz",
      "sha256": "3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b"
    }
  ]
}
```

Field rules the client enforces:

- `schema` must be `1` — anything else is refused.
- **`sha256` is mandatory and always verified.** A missing checksum is a
  hard error, not a warning; a mismatch refuses the install and leaves
  the cache clean. There is no "skip verification" switch.
- One entry **per version** (publish a new index entry per release);
  the client sorts versions semver-wise and resolves `latest` from that.
- `tarball` is relative to the base URL (absolute URLs also work).
- **`guppy-template` keyword is required for `guppy template search`
  visibility** (npm/topic parity). Entries without it are installable by
  explicit id but never show up in search.

## The tarball

Release archives contain the template directory — either flat or wrapped
in a single top-level directory (the client strips exactly one wrapping
level):

```text
guppy-my-template-1.2.0.tar.gz
└── my-template/
    ├── guppy.toml
    ├── tests.toml
    └── template/...
```

Extraction refuses absolute paths, `..` escapes, symlinks, hardlinks,
and special files; entries are capped at 32 MiB. Build it straight from your authoring directory:

```sh
tar -czf guppy-my-template-1.2.0.tar.gz my-template
sha256sum guppy-my-template-1.2.0.tar.gz   # paste into index.json
```

Validate before publishing — the same commands consumers' CI would run:

```sh
guppy dev lint ./my-template && guppy dev test ./my-template
```

## Publishing a new version

1. Bump `version` in the template's `guppy.toml` (semver).
2. `guppy dev lint && guppy dev test` green locally.
3. Build the new tarball; compute its sha256.
4. Append the new `Entry` to `index.json` (keep old entries — installs
   pin `id@version` and older pins must keep resolving).
5. Upload tarball + index.

For the official registry this whole flow is a PR:
[nicedata-work/guppy-registry](https://github.com/nicedata-work/guppy-registry)
holds `index.json` only — your template and tarball stay in your own
repo — and its CI fetches the tarball, verifies the checksum, and runs
`guppy dev lint` + `guppy dev test` before a maintainer merges. See that repo's
[CONTRIBUTING.md](https://github.com/nicedata-work/guppy-registry/blob/main/CONTRIBUTING.md).

Consumers then run `guppy template update <id>` (or `--all`): the client
resolves the newest indexed version and shows a unified diff for review.

## The trust model (what consumers experience)

Templates are installed into the **community** scope only through
`guppy template add community:<source>` (id, `registry:id@ver`, or
`git:…@ref`, each with the `community:` qualifier). The client:

- prints the manifest, the file list, and (for upgrades) the diff;
- requires an interactive **review + confirm** before the template is
  usable for the first time — the trust record lives in the installs
  ledger (`<data>/guppy/installs.toml`), and *no record, no trust*;
- **re-enters the gate on every new version** — publishing a version is
  a new trust decision for consumers, not a silent update;
- lets users revoke with `guppy template remove community:<id>`.

As a publisher this means: keep diffs reviewable, keep `description` and
`keywords` current, and never reuse a version number for different
content — the index is append-only and consumers diff against it.

## Hosting notes

- Any static server works; HTTPS is required unless the host is added to
  `[network] insecure` in consumer settings.
- `--offline` consumers can only resolve already-installed community
  copies — publishing cannot break offline scaffolding.
- The Guppy project's own release artifacts (the binary) live next to,
  but separate from, any template registry; see `scripts/release.sh`.
