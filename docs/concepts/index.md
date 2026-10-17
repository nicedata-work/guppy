# Concepts

Explanations of how guppy works and why. Guides tell you *how* to do things;
these pages explain the model behind them.

- [Template model](template-model.md) — layout, the `guppy.toml` manifest, and the deliberately small renderer
- [Scopes, resolution, and identity](scopes-and-resolution.md) — where templates live, how names resolve, what makes a template *the same* template
- [Extension (`extends`)](extension.md) — declarative inheritance, never executable hooks
- [The project CLI (`gup`)](project-cli.md) — one binary, two faces; the `[gup]` command map
- [Doctor and the catalog](doctor-and-catalog.md) — the project marker, the global index, and the shellhook
- [Trust and security](trust-and-security.md) — the trust boundary and what guppy refuses to do
