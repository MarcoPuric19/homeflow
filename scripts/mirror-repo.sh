#!/usr/bin/env sh
# Spiegelt die Repository-Historie auf jeden konfigurierten Zweitdatenträger.
#
# Hintergrund steht in docs/adr/0008-ci-cd-location.md: die Historie existierte
# genau einmal, auf einem Laptop. Dieses Skript ist die Gegenmaßnahme - und
# bewusst ehrlich statt bequem. Kein Spiegel konfiguriert heißt Fehlschlag, nicht
# stiller Erfolg; ein nicht eingehängter Datenträger heißt Fehlschlag, nicht
# "war wohl nichts zu tun".
#
# Ein Spiegel ist ein gewöhnliches git-Remote, dessen Name mit `spiegel`
# beginnt. Anlegen zum Beispiel so:
#
#     git init --bare /media/$USER/backup/homeflow.git
#     git remote add spiegel-usb /media/$USER/backup/homeflow.git
#
# Es dürfen mehrere sein. Was das Skript NICHT leistet: es sichert keine
# Arbeitskopie und keine Daten. Nicht Committetes ist nicht gespiegelt - darauf
# weist es am Ende ausdrücklich hin. Für PostgreSQL und Belege ist
# scripts/backup.sh zuständig.
set -eu

cd "$(git rev-parse --show-toplevel)"

DRY=0
LEISE=0
case "${1:-}" in
  "")        ;;
  --pruefen) DRY=1 ;;
  # Für den post-commit-Hook: spiegelt, was erreichbar ist, sagt bei Erfolg
  # nichts und scheitert nie. Ein blockierter Commit ist ein Grund, Hooks
  # abzuschalten - dann ist gar nichts mehr gesichert.
  --leise)   LEISE=1 ;;
  *)
    echo "Aufruf: $0 [--pruefen|--leise]" >&2
    echo "  --pruefen  meldet nur den Stand und schreibt nichts" >&2
    echo "  --leise    für Hooks: still bei Erfolg, nie ein Fehlschlag" >&2
    exit 64 ;;
esac

spiegel=$(git remote | grep '^spiegel' || true)
if [ -z "$spiegel" ] && [ "$LEISE" -eq 1 ]; then
  exit 0
fi
if [ -z "$spiegel" ]; then
  echo "Kein Spiegel konfiguriert." >&2
  echo "" >&2
  echo "Die Historie liegt damit genau einmal, auf dieser Maschine." >&2
  echo "Anlegen (Pfad anpassen):" >&2
  echo "" >&2
  echo "  git init --bare /media/\$USER/<datentraeger>/homeflow.git" >&2
  echo "  git remote add spiegel-usb /media/\$USER/<datentraeger>/homeflow.git" >&2
  exit 69
fi

lokal=$(git rev-parse HEAD)
zweig=$(git rev-parse --abbrev-ref HEAD)
fehler=0

for remote in $spiegel; do
  ziel=$(git remote get-url "$remote")

  # Erreichbarkeit zuerst prüfen, damit ein nicht eingehängter Datenträger als
  # das gemeldet wird, was er ist, statt als git-Fehlermeldung.
  case "$ziel" in
    /*|file://*)
      pfad=${ziel#file://}
      if [ ! -d "$pfad" ]; then
        [ "$LEISE" -eq 0 ] && echo "FEHLT     $remote -> $ziel (nicht eingehängt oder gelöscht)"
        fehler=1
        continue
      fi
      ;;
  esac

  if ! git ls-remote --heads "$remote" >/dev/null 2>&1; then
    [ "$LEISE" -eq 0 ] && echo "FEHLER    $remote -> $ziel (nicht erreichbar)"
    fehler=1
    continue
  fi

  fern=$(git ls-remote "$remote" "refs/heads/$zweig" 2>/dev/null | cut -f1)
  if [ "$fern" = "$lokal" ]; then
    [ "$LEISE" -eq 0 ] && echo "AKTUELL   $remote ($zweig auf $(echo "$lokal" | cut -c1-8))"
    continue
  fi

  offen=$(git rev-list --count "${fern:-}${fern:+..}HEAD" 2>/dev/null || git rev-list --count HEAD)
  if [ "$DRY" -eq 1 ]; then
    echo "OFFEN     $remote: $offen Commit(s) nicht gespiegelt"
    fehler=1
    continue
  fi

  # --mirror, nicht --all: der Spiegel soll den lokalen Stand abbilden,
  # einschließlich gelöschter Zweige. Er ist eine Kopie, kein zweiter Verlauf.
  if git push --mirror "$remote" >/dev/null 2>&1; then
    [ "$LEISE" -eq 0 ] && echo "GESPIEGELT $remote: $offen Commit(s) übertragen"
  else
    [ "$LEISE" -eq 0 ] && echo "FEHLER    $remote: push abgelehnt"
    fehler=1
  fi
done

# Der eigentliche Hinweis kommt zum Schluss, weil er meistens der wichtigere
# ist: gespiegelt wird nur, was committet ist.
schmutzig=$(git status --porcelain | wc -l)
if [ "$schmutzig" -gt 0 ] && [ "$LEISE" -eq 0 ]; then
  echo ""
  echo "ACHTUNG: $schmutzig Datei(en) im Arbeitsbaum sind nicht committet und"
  echo "damit auch nicht gespiegelt. Ein Spiegel ist nur so gut wie der letzte"
  echo "Commit."
fi

# Im Hook-Betrieb ist ein fehlender Spiegel kein Grund, den Commit rot zu
# machen. Sichtbar wird der Rückstand in Mission Control, nicht hier.
[ "$LEISE" -eq 1 ] && exit 0
exit $fehler
