# guppy completion powershell

## Synopsis

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	guppy completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

## Usage

```
guppy completion powershell [flags]
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

