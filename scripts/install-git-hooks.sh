#!/usr/bin/env sh
# Installs the optional pre-push hook. Run it deliberately; nothing installs it
# for you. Remove it again with: rm .git/hooks/pre-push
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

hook_dir=$(git rev-parse --git-path hooks)
source_hook=scripts/pre-push.sh
target_hook="$hook_dir/pre-push"

test -f "$source_hook" || { echo "Missing $source_hook" >&2; exit 1; }
mkdir -p "$hook_dir"

if [ -e "$target_hook" ] && ! cmp -s "$source_hook" "$target_hook"; then
  if [ "${1:-}" != "--force" ]; then
    echo "Refusing: $target_hook exists and differs. Inspect it, then rerun with --force." >&2
    exit 1
  fi
  cp "$target_hook" "$target_hook.bak"
  echo "Existing hook kept as $target_hook.bak"
fi

cp "$source_hook" "$target_hook"
chmod +x "$target_hook"
echo "Installed $target_hook - every 'git push' now runs 'make ci' first."
if [ -z "$(git remote)" ]; then
  echo "Note: this repository has no remote, so the hook stays dormant until one exists."
fi
