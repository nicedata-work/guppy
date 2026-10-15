#!/usr/bin/env python3
"""Builds the registry from templates/: index.json + tarballs/.

Every directory under templates/ containing a guppy.toml is one
published template version (the version comes from its manifest). The
build packs each one into tarballs/guppy-<id>-<version>.tar.gz (a
single top-level <id>/ directory — exactly what guppy's client
strips), records the sha256, and regenerates index.json (schema 1).

Stale tarballs (template removed, or a version bumped without a new
entry) are pruned: this repo's checkout is always a complete, current
registry, ready to be consumed via [[template.registries]].

Usage: build-registry.py [--check]
    (default) build: (re)write index.json and tarballs/
    --check   exit 1 if the on-disk registry is stale, without writing
"""
import gzip
import hashlib
import io
import json
import os
import sys
import tarfile
import tomllib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TEMPLATES_DIR = os.path.join(ROOT, "templates")
TARBALLS_DIR = os.path.join(ROOT, "tarballs")
INDEX = os.path.join(ROOT, "index.json")
KEYWORD = "guppy-template"


def fail(msg):
    print(f"error: {msg}", file=sys.stderr)
    sys.exit(1)


def collect():
    """Returns [(dir, manifest)] for every template under templates/."""
    out = []
    if not os.path.isdir(TEMPLATES_DIR):
        return out
    for name in sorted(os.listdir(TEMPLATES_DIR)):
        d = os.path.join(TEMPLATES_DIR, name)
        manifest_path = os.path.join(d, "guppy.toml")
        if not os.path.isfile(manifest_path):
            continue
        with open(manifest_path, "rb") as f:
            m = tomllib.load(f)
        if not m.get("id") or not m.get("version"):
            fail(f"templates/{name}: guppy.toml needs id and version")
        out.append((d, m))
    return out


def build_tarball(src_dir, template_id):
    """Packs src_dir as <template_id>/… into a tar.gz; returns bytes.

    Deterministic: sorted entries, zeroed mtimes/uid/gid — same inputs
    produce byte-identical tarballs, so CI rebuilds are no-op diffs.
    """
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w") as tar:
        for base, dirs, files in os.walk(src_dir):
            dirs.sort()
            for f in sorted(files):
                path = os.path.join(base, f)
                arcname = os.path.join(template_id, os.path.relpath(path, src_dir))
                info = tar.gettarinfo(path, arcname=arcname)
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                info.mtime = 0
                with open(path, "rb") as fh:
                    tar.addfile(info, fh)
    gz = io.BytesIO()
    with gzip.GzipFile(fileobj=gz, mode="wb", mtime=0) as gzout:
        gzout.write(raw.getvalue())
    return gz.getvalue()


def desired():
    """Returns (index_dict, {tarball_name: bytes}) for templates/."""
    templates = []
    tarballs = {}
    for d, m in collect():
        tid, version = m["id"], m["version"]
        data = build_tarball(d, tid)
        name = f"guppy-{tid}-{version}.tar.gz"
        tarballs[name] = data
        templates.append({
            "id": tid,
            "version": version,
            "kind": m.get("kind", ""),
            "language": m.get("language", ""),
            "description": m.get("description", ""),
            "keywords": [KEYWORD, m.get("kind", ""), m.get("language", "")],
            "tarball": f"tarballs/{name}",
            "sha256": hashlib.sha256(data).hexdigest(),
        })
    return {"schema": 1, "templates": templates}, tarballs


def current_tarballs():
    names = {}
    if os.path.isdir(TARBALLS_DIR):
        for f in sorted(os.listdir(TARBALLS_DIR)):
            if f.endswith(".tar.gz"):
                names[f] = open(os.path.join(TARBALLS_DIR, f), "rb").read()
    return names


def main():
    check = "--check" in sys.argv[1:]
    index, tarballs = desired()

    if check:
        try:
            on_disk = json.load(open(INDEX))
        except (OSError, json.JSONDecodeError):
            print("registry is stale: index.json missing or unparsable", file=sys.stderr)
            sys.exit(1)
        stale = on_disk != index or current_tarballs() != tarballs
        if stale:
            print("registry is stale — run: guppy project build "
                  "(python3 scripts/build-registry.py)", file=sys.stderr)
            sys.exit(1)
        print("registry is up to date")
        return

    os.makedirs(TARBALLS_DIR, exist_ok=True)
    for name, data in tarballs.items():
        with open(os.path.join(TARBALLS_DIR, name), "wb") as f:
            f.write(data)
    for name in set(current_tarballs()) - set(tarballs):
        os.remove(os.path.join(TARBALLS_DIR, name))  # stale version tarballs
    with open(INDEX, "w") as f:
        json.dump(index, f, indent=2)
        f.write("\n")
    print(f"registry: {len(tarballs)} template(s) indexed")


if __name__ == "__main__":
    main()
