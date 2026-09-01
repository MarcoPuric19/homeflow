#!/usr/bin/env sh
# HomeFlow pre-push hook: runs the same checks as `make ci` before a push.
#
# Installed by `make install-git-hooks`, never automatically.
# Skip once with `git push --no-verify` or `HOMEFLOW_SKIP_CI=1 git push`.
set -eu

if [ "${HOMEFLOW_SKIP_CI:-0}" = "1" ]; then
  echo "pre-push: skipped because HOMEFLOW_SKIP_CI=1" >&2
  exit 0
fi

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

echo "pre-push: running 'make ci' (skip with --no-verify)" >&2
if make ci; then
  exit 0
fi

echo "pre-push: 'make ci' failed, push aborted." >&2
exit 1
