#!/bin/sh
# Emits bazel workspace status keys (used by .bazelrc workspace_status_command).
# Kept trivial and always-succeeding: status failures must not break builds.

echo "STABLE_GIT_COMMIT $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "STABLE_GIT_DESCRIBE $(git describe --always --dirty 2>/dev/null || echo unknown)"
