#!/usr/bin/env sh
# Richtet den Nahspiegel ein: Bare-Repo anlegen, als Remote eintragen, Hooks
# installieren, einmal spiegeln. Ein Befehl statt vier (HF-0106).
#
# Der Nahspiegel liegt auf demselben Dateisystem und schützt deshalb NICHT gegen
# Plattenausfall. Er schützt gegen überschriebene Dateien, verunglückte Rebases
# und ein zerschossenes .git - genau das, was hier zweimal passiert ist
# (HF-0010). Für Redundanz gegen Hardwareausfall braucht es einen zweiten
# Datenträger; das Skript sagt das am Ende noch einmal.
set -eu

cd "$(git rev-parse --show-toplevel)"

: "${MIRROR_PATH:=$HOME/.local/share/homeflow-mirror.git}"
: "${MIRROR_REMOTE:=spiegel-lokal}"

if [ $# -gt 0 ]; then
  echo "Aufruf: $0" >&2
  echo "  MIRROR_PATH=$MIRROR_PATH" >&2
  echo "  MIRROR_REMOTE=$MIRROR_REMOTE" >&2
  exit 64
fi

if [ -d "$MIRROR_PATH" ]; then
  echo "Vorhanden: $MIRROR_PATH"
else
  mkdir -p "$(dirname "$MIRROR_PATH")"
  git init --bare -q "$MIRROR_PATH"
  echo "Angelegt:  $MIRROR_PATH"
fi

if git remote get-url "$MIRROR_REMOTE" >/dev/null 2>&1; then
  vorhanden=$(git remote get-url "$MIRROR_REMOTE")
  if [ "$vorhanden" != "$MIRROR_PATH" ]; then
    # Nicht stillschweigend umbiegen: ein Remote, das woandershin zeigt, ist
    # vielleicht der USB-Spiegel und nicht der hier gemeinte.
    echo "Remote $MIRROR_REMOTE zeigt bereits auf $vorhanden - unverändert gelassen." >&2
    exit 1
  fi
  echo "Remote:    $MIRROR_REMOTE (bereits eingetragen)"
else
  git remote add "$MIRROR_REMOTE" "$MIRROR_PATH"
  echo "Remote:    $MIRROR_REMOTE -> $MIRROR_PATH"
fi

./scripts/install-git-hooks.sh >/dev/null
echo "Hooks:     pre-push und post-commit installiert"

./scripts/mirror-repo.sh

cat <<'HINWEIS'

Der Nahspiegel liegt auf derselben Platte. Er rettet vor Fehlgriffen, nicht vor
Hardwareausfall. Für echte Redundanz einen zweiten Datenträger einhängen und
zusätzlich eintragen:

    git init --bare /media/$USER/<datentraeger>/homeflow.git
    git remote add spiegel-usb /media/$USER/<datentraeger>/homeflow.git

Wie weit ein Spiegel zurückliegt, steht in Mission Control in der Repo-Ansicht.
HINWEIS
