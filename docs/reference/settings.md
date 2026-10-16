# Settings

User settings live in one TOML file, `~/.config/guppy/guppy.toml` — flat,
commented, every key env-overridable.

```toml
# Templates
[defaults]                 # user-level overrides of template [defaults];
                           # still beaten by --no-* flags per invocation
linters = true

[template]                 # template sourcing
dirs     = ["~/src/my-templates"]   # extra user-scope dirs
registry = "https://raw.githubusercontent.com/nicedata-work/guppy-registry/main"

# Named team registries, searched in declaration order BEFORE the default.
# Git registries clone with the user's own git credentials — no tokens:
[[template.registries]]
name = "team"
git = "git@ssh.dev.azure.com:v3/acme/pipeline-templates"
# ref = "main"      # default: HEAD
# trust = "full"    # skip the review gate for registries your org controls
# Or an HTTP-served index (pair with GUPPY_REGISTRY_TEAM_TOKEN):
# [[template.registries]]
# name = "http-team"
# url = "https://registry.internal.acme/guppy"

[shell]                    # shellhook
hook = true                # emit cd/prompt hook (guppy shell-env installs it)

[network]
offline  = false
insecure = []              # allowed-insecure-host list
```

## Keys

| Section | Key | Default | Description |
|---|---|---|---|
| `[defaults]` | `linters`, `bazel`, `direnv`, `git`, `tests`, `agents_md` | from template | user-level override of template default toggles |
| `[template]` | `dirs` | `[]` | extra user-scope template directories |
| `[template]` | `registry` | `https://raw.githubusercontent.com/nicedata-work/guppy-registry/main` | default registry index URL |
| `[template]` | `registries` | `[]` | named team registries: `name`, `git` (+ optional `ref`) or `url`, `trust` (`review` default, `full` skips the review gate); searched in declaration order before `registry` |
| `[shell]` | `hook` | `true` | whether `guppy shell-env` installs the cd/prompt hook |
| `[network]` | `offline` | `false` | default for `--offline` |
| `[network]` | `insecure` | `[]` | hosts allowed to use insecure transport |

## Precedence

The uv convention: **defaults < `guppy.toml` < env (`GUPPY_*`) < flags**.
A user-level `[defaults]` setting beats a template's default; a per-invocation
`--no-*` flag beats both.

## Project settings are separate

Project settings live only in `.guppy/project.toml` and affect the project
face (`gp`). The global CLI never reads project state to change its own
behavior — that is the trust boundary of
[Trust and security](../concepts/trust-and-security.md).

## Where the file is

`~/.config/guppy/guppy.toml` on Linux/macOS, `%APPDATA%\guppy\guppy.toml`
on Windows, overridable with the standard `XDG_CONFIG_HOME` and `--config`.
See [Paths and storage](paths.md).
