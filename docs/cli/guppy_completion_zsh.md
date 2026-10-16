# guppy completion zsh

## Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(guppy completion zsh)

To load completions for every new session, execute once:

#### Linux:

	guppy completion zsh > "${fpath[1]}/_guppy"

#### macOS:

	guppy completion zsh > $(brew --prefix)/share/zsh/site-functions/_guppy

You will need to start a new shell for this setup to take effect.

## Usage

```
guppy completion zsh [flags]
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

