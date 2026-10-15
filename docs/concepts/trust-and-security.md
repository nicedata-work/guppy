# Trust and security

Guppy scaffolds directories of files from remote sources, so the trust
model is a first-class constraint. This page is what you experience as a
user; what it means for your templates is summarized at the end.

## The rules, in brief

- **Community templates are an explicit, per-template decision.** A
  `community:` install prints the manifest, file list, and diff, and
  requires interactive review-and-confirm before the template is usable.
  Every update re-enters the gate — a new version is a new trust decision.
  First-party templates never require confirmation.
- **Built-ins always win resolution.** A community template cannot shadow a
  first-party id; collisions warn and resolve deterministically.
- **No executable code in templates.** Templates are declarative data
  (`guppy.toml` + payload). There is no plugin runtime and no hook story;
  the renderer is a pure function with no filesystem access.
- **The global CLI never reads project state to change its own behavior.**
  Project settings live only in `.guppy/project.toml` and affect the
  project face (`gp`) — a scaffolded repository cannot change how `guppy`
  itself behaves.
- **Checksums everywhere.** Release tarballs, registry artifacts, and
  self-updates are checksum-verified; `guppy update` restores the previous
  binary on failure.
- **Bounded and traversal-safe.** Payload materialization is symlink-safe,
  size-capped, and refuses path traversal and special files; parsers are
  hardened against untrusted manifests.
- **Scoped, revocable footprint.** `guppy uninstall` removes the binary,
  rc-file hook lines, and data directory; nothing is written outside XDG
  paths and the project tree.

## Why the boundary is shaped this way

The one cross-boundary flow — installing and running a community template —
is gated at every step: source pinned at add time (git SHAs, registry
versions), contents reviewed before first use, rendering sandboxed, output
materialized defensively. Everything guppy *generates* is data; the only
code that ever runs is guppy itself and the consuming project's own pinned
Bazel targets.

## Reporting

Security issues are reported privately through GitHub's security
advisories on the guppy repository — do not open public issues for
security problems.
