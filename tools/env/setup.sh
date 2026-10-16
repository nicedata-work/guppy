# tools/env/setup.sh — activate the hermetic environment for the guppy
# repository (environment contract, design §3.4; same shape as the
# base-toolchain payload). Sourced from .envrc; idempotent, safe on every
# activation, never writes outside the project tree.
ROOT="${GUPPY_ROOT:-$PWD}"

if ! has bazel; then
  log_error "bazel not found — install bazelisk: https://bazel.build/install/bazelisk"
fi

# The shim dir: project-scoped shims live here, never on the global PATH.
# The gp shim — the project face of guppy (§3.5) — is materialized on
# every activation, so it always points at the guppy that `command -v`
# finds here (the fresh build, when one exists) and exists only while
# this project's environment is active.
SHIM_DIR="$ROOT/.direnv/bin"
mkdir -p "$SHIM_DIR"
PATH_add "$SHIM_DIR"

# The build output dir is added last, so it sits *before* everything else
# on PATH: inside this repo `guppy` and `gp` resolve to the binary
# `bazel build //...` produced — no copy/install step. The dir only
# exists after a build; without one the entry is skipped and the
# installed guppy wins as before.
BUILD_BIN_DIR="$ROOT/.bazel/bin/cmd/guppy/guppy_"
if [ -d "$BUILD_BIN_DIR" ]; then
  PATH_add "$BUILD_BIN_DIR"
fi

if has guppy; then
  ln -sfn "$(command -v guppy)" "$SHIM_DIR/gp"
else
  log_status "guppy not found — gp shim unavailable (fallback: guppy project <verb>)"
fi

export GUPPY_REPO_ROOT="$ROOT"
