"""Run ruff from the pip hub.

ruff ships its CLI as a wheel-installed binary (no console_scripts entry
point), so instead of `python -m ruff`'s sysconfig probing — which cannot
see Bazel runfiles — the binary is resolved relative to the package and
exec'd directly. `@pypi//ruff:data` puts bin/ruff into the runfiles tree.
"""

import os
import sys

import ruff

ruff_bin = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(ruff.__file__))), "bin", "ruff"
)
os.execv(ruff_bin, [ruff_bin, *sys.argv[1:]])
