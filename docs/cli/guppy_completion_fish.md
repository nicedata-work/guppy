# guppy completion fish

## Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	guppy completion fish | source

To load completions for every new session, execute once:

	guppy completion fish > ~/.config/fish/completions/guppy.fish

You will need to start a new shell for this setup to take effect.

## Usage

```
guppy completion fish [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| --no-descriptions | disable completion descriptions |


## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

