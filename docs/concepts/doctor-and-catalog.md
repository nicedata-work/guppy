# Doctor and the catalog

Every project guppy manages is traceable, and traceability is what powers
`guppy doctor`, `guppy project list`, and cheap template upgrades. Two artifacts do
the work: a per-project marker and a global index.

## The project marker

**`.guppy/project.toml`** is written at scaffold time and records:

- the template identity and version, and the resolved `extends` chain;
- the var values and resolved `[defaults]` toggles (what doctor re-renders
  the expected file set from);
- the `[gp]` command map;
- the guppy version and a `schema` version for forward compatibility.

The file is deterministic — no timestamps, sorted keys — so idempotent
re-scaffolds keep it byte-identical. It is committed to the repo (small, no
secrets) so provenance survives clones.

## The global index

**`~/.local/share/guppy/index.toml`** indexes every managed project on the
machine. It is maintained by a **shellhook** (`guppy shell-env`), not a
daemon or filesystem watcher:

- on `cd`/prompt, the hook checks the current directory (one
  `test -f .guppy/project.toml`) and appends the path to an append-only
  seen-log — only when the directory changed;
- the hook is pure shell and **never invokes guppy**: failure degrades to a
  stale index, never to a broken shell;
- index readers (`guppy project list`, `guppy doctor`, `guppy new`) fold the log
  into `index.toml` off the hot path and clear it after a successful fold —
  a failed fold is retried, never lost;
- readers prune entries whose path or marker no longer exists, so the index
  self-heals without manual GC.

This is monitoring-by-presence: the marker is the truth; the index is a
cache of where markers have been seen.

## `guppy doctor`

Doctor is manifest-driven and read-only. Given a project directory it:

1. locates `.guppy/project.toml` and the template identity it records;
2. diffs the project tree against the template's expected file set —
   missing/renamed/deleted payload files, stale pins (`.bazelversion`
   versus the template's current default);
3. emits recommendations.

`--fix` applies the safe subset: restoring missing pinned files never
touches user-edited content. `guppy doctor --all` batches across every
project in the index.

## Why this pays for itself

Because scaffolds always write the marker, upgrades are mechanical:
`guppy template update` + `guppy doctor` can tell a user precisely which
scaffold files a newer template version would change — provenance at
creation, diffable after.
