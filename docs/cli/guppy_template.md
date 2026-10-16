# guppy template

## Synopsis

Inspect and manage templates: show merged views, list/search the catalog, install (add), uninstall (remove), copy out for editing (clone), reconcile installs with their sources (update), and drop the download cache (refresh).

## Usage

```
guppy template
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

* [guppy template add](guppy_template_add.md)	Install a template from the registry, git, or a local path
* [guppy template clone](guppy_template_clone.md)	Copy a template out to a local directory for editing
* [guppy template list](guppy_template_list.md)	List installed templates (built-in, user, community)
* [guppy template refresh](guppy_template_refresh.md)	Refresh the registry cache (drop cached downloads)
* [guppy template remove](guppy_template_remove.md)	Uninstall a template and forget its provenance
* [guppy template search](guppy_template_search.md)	Search the configured registries
* [guppy template show](guppy_template_show.md)	Show a template's merged view: chain, vars, defaults, and payload
* [guppy template update](guppy_template_update.md)	Reconcile installed templates with their sources (all, by default)
