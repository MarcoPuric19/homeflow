#!/usr/bin/env sh
# Installs the optional git hooks. Run it deliberately; nothing installs them
# for you. Remove them again with: rm .git/hooks/pre-push .git/hooks/post-commit
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

hook_dir=$(git rev-parse --git-path hooks)
mkdir -p "$hook_dir"

install_hook() {
  source_hook=$1
  target_hook="$hook_dir/$2"

  test -f "$source_hook" || { echo "Missing $source_hook" >&2; exit 1; }

  if [ -e "$target_hook" ] && ! cmp -s "$source_hook" "$target_hook"; then
    if [ "${FORCE:-}" != "1" ]; then
      echo "Refusing: $target_hook exists and differs. Inspect it, then rerun with --force." >&2
      exit 1
    fi
    cp "$target_hook" "$target_hook.bak"
    echo "Existing hook kept as $target_hook.bak"
  fi

  cp "$source_hook" "$target_hook"
  chmod +x "$target_hook"
}

[ "${1:-}" = "--force" ] && FORCE=1 || FORCE=0
export FORCE

install_hook scripts/pre-push.sh pre-push
echo "Installed $hook_dir/pre-push - every 'git push' now runs 'make ci' first."

# Der post-commit-Hook ist der eigentliche Schutz auf dieser Maschine: er
# spiegelt nach jedem Commit, ohne dass jemand daran denken muss (ADR 0008).
install_hook scripts/post-commit.sh post-commit
echo "Installed $hook_dir/post-commit - every commit now mirrors quietly."

if [ -z "$(git remote)" ]; then
  echo "Note: no remote and no mirror configured, so both hooks stay dormant."
  echo "      'make mirror-setup' creates the local mirror and wires it up."
fi
