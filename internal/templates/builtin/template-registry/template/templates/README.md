# templates/ — the drop zone

One directory per template, named after its id. Each contains:

```text
templates/<id>/
├── guppy.toml            # manifest: id, version, kind, language, [vars], [defaults], [gup]
├── tests.toml            # render-matrix cases for guppy test
├── testdata/snapshot/    # golden snapshots (committed — CI runs guppy test)
└── template/             # the payload; .hbs files are rendered, the rest copied verbatim
```

The version in each `guppy.toml` **is** the published version: bump it
to release, and CI publishes a new `index.json` entry + tarball for it.

Reference: the
[authoring guide](https://github.com/nicedata-work/guppy/blob/main/docs/guides/authoring-templates.md)
and the first-party templates in the
[guppy repo](https://github.com/nicedata-work/guppy/tree/main/internal/templates/builtin)
are good starting points (start from `base-toolchain` structure, copy
what you need — templates are plain directories).
