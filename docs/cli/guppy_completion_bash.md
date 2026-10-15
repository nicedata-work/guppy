# guppy completion bash

## Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(guppy completion bash)

To load completions for every new session, execute once:

#### Linux:

	guppy completion bash > /etc/bash_completion.d/guppy

#### macOS:

	guppy completion bash > $(brew --prefix)/etc/bash_completion.d/guppy

You will need to start a new shell for this setup to take effect.

## Usage

```
guppy completion bash [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| --no-descriptions | disable completion descriptions |


## Global flags

| Flag | Description |
|------|-------------|
| --color (default auto) | color output: auto, always, never |
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --no-progress | disable progress output |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |

