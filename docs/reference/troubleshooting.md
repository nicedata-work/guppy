# Troubleshooting

## `gp: command not found`

`gp` is linked into `.direnv/bin/` by the project's direnv setup, so it
exists only while direnv has activated the project. Check, in order:

1. direnv is installed and hooked into your shell (`direnv status`).
2. You have allowed the project's `.envrc` (`direnv allow`).
3. `guppy doctor` flags a missing direnv hook or out-of-sync `.envrc` —
   run it and follow its recommendations.

Until fixed, the canonical form works from anywhere:
`guppy project test`.

## `guppy new` says the template choice is ambiguous

With no `--template`, guppy picks from the kind × language matrix and fails
with a candidate list when the choice is ambiguous. Pass `--kind`, `--lang`,
or `--template <id>` explicitly — see
[Creating projects](../guides/creating-projects.md).

## `guppy template add community:…` waits for confirmation

That is the trust gate: community templates require an interactive
review-and-confirm before first use (and again on every update). In
non-interactive contexts pass `--yes` explicitly. First-party templates
never require confirmation.

## A var referenced in the payload is reported as a lint error

The substitution allow-list is strict: an undeclared `{{ .X }}` is an
error, never silently empty. Declare the var in `[vars]` (or remove the
reference). A declared-but-unused var is only a warning.

## The project index (`guppy list`) is stale or missing entries

The index is maintained by the shellhook (`guppy init-shell`) and folded
off the hot path; a failed fold is retried on the next read. Entries whose
path or marker no longer exists are pruned automatically. If entries are
missing, re-run `guppy init-shell` and confirm the eval line is in your rc
file; a broken hook never breaks your shell — it only leaves the index
stale.

## `guppy update` did not update templates

By design: `guppy update` manages the executable only. Templates are
reconciled separately with `guppy template update`.

## Registry/network errors

- Confirm connectivity; error exit code is `3`.
- `--offline` / `GUPPY_OFFLINE=true` disables all network sources —
  built-in, user, and already-installed community templates still resolve.
- `[network].insecure` in `guppy.toml` lists hosts allowed to use insecure
  transport (see [Settings](settings.md)).

## Scaffolding into a non-empty directory was refused

Existing-tree safety: a non-empty directory is refused (exit 2) unless
`--force` is passed. Even then the merge never clobbers — differing files
are kept and reported.

## Still stuck

See [Getting help](../getting-started/help.md) — and include `-v` output,
`guppy --version`, and (for template issues) `guppy template show <id>`.
