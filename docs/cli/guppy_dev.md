# guppy dev

## Synopsis

Develop guppy templates: the authoring loop of design §7.

guppy dev new scaffolds a minimal template shell to start from; dev
lint and dev test verify a template statically and by render snapshot;
dev publish prepares the registry contribution (tarball + index entry).
Local templates are developed in place — dev list shows the ones the
install ledger knows about.

## Usage

```
guppy dev
```

## Global flags

| Flag | Description |
|------|-------------|
| --config | path to guppy.toml settings file |
| --directory | run as if started in this directory |
| --interactive | enable interactive prompts |
| --offline | disable all network access |
| -q, --quiet | only print errors |
| -v, --verbose | increase output verbosity |


## See also

* [guppy dev lint](guppy_dev_lint.md)	Lint a template: manifest, extends chain, and payload substitutions
* [guppy dev list](guppy_dev_list.md)	List locally developed templates
* [guppy dev new](guppy_dev_new.md)	Create a new template shell to start authoring from
* [guppy dev publish](guppy_dev_publish.md)	Prepare a template's registry contribution (tarball + index entry)
* [guppy dev test](guppy_dev_test.md)	Render snapshot tests for a template (golden files, var matrices)
