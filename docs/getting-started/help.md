# Getting help

## Questions and bugs

Open an issue on the [issue tracker](https://github.com/nicedata-work/guppy/issues).

When reporting a bug, include:

- the exact command and full output (run with `-v` for verbose logging);
- your OS/arch and `guppy --version`;
- for template issues, the template id/version from
  `guppy template show <id>` and, if scaffolding failed, the `guppy dev lint`
  output for it;
- what you expected to happen.

Reproducible examples are gold: a minimal template directory that triggers
the bug plus the command that fails is usually enough.

## Common problems

Many issues have known fixes — see
[Troubleshooting](../reference/troubleshooting.md) first: `gp` not found,
ambiguous templates, trust-gate prompts in CI, stale index entries, and
offline use are all covered there.

## Security issues

Do **not** open a public issue for security problems — report through
GitHub's private security advisories on the guppy repository.

## CLI help

Every command documents itself — `guppy --help` and
`guppy <command> --help` — and the generated
[command reference](../cli/guppy.md) mirrors it.
