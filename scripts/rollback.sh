#!/usr/bin/env sh
# Rollt eine Pipeline auf einen frueheren Commit zurueck - ohne den Arbeitsbaum
# anzufassen.
#
# Das ist der Punkt, an dem HF-0104 haengengeblieben ist: ein Rueckbau braucht
# die Manifeste von damals, und die holt man ueblicherweise mit einem Checkout.
# Neben einem Assistenten, der gerade an denselben Dateien arbeitet, ist das
# gefaehrlich - und ein Rollback greift man genau dann, wenn ohnehin schon etwas
# kaputt ist.
#
# `git archive` liest aus der Objektdatenbank in ein Wegwerf-Verzeichnis. Der
# Arbeitsbaum wird nicht gelesen, nicht geschrieben, nicht gestasht. Damit
# faellt das Risiko weg, das im Ticket beschrieben steht.
#
# Was dieses Skript ausdruecklich NICHT kann: Datenbankmigrationen zuruecknehmen.
# Ein Rollback ueber eine Migration hinweg braucht die .down.sql und eine
# bewusste Entscheidung - das ist ein eigener Vorgang, kein Knopf.

set -eu

PIPELINE="${1:-}"
COMMIT="${2:-}"
DRY="${ROLLBACK_DRY_RUN:-0}"

rot() { printf '\033[31m%s\033[0m\n' "$*" >&2; }

if [ -z "$PIPELINE" ] || [ -z "$COMMIT" ]; then
  rot "Aufruf: $0 <pipeline> <commit>"
  rot "  pipeline: homeflow | monitoring"
  rot "  commit:   der Stand, auf den zurueckgerollt wird (aus dem Deploy-Journal)"
  exit 64
fi

wurzel="$(cd "$(dirname "$0")/.." && pwd)"
cd "$wurzel"

if ! git rev-parse --verify --quiet "$COMMIT^{commit}" >/dev/null; then
  rot "Unbekannter Commit: $COMMIT"
  exit 65
fi
aufloesung="$(git rev-parse --short "$COMMIT")"

case "$PIPELINE" in
  homeflow)   pfad="kubernetes"; kustomize="kubernetes/base"; namespace="homeflow" ;;
  monitoring) pfad="monitoring"; kustomize="monitoring/kubernetes"; namespace="homeflow-monitoring" ;;
  *) rot "Unbekannte Pipeline: $PIPELINE (homeflow oder monitoring)"; exit 64 ;;
esac

if ! git cat-file -e "$COMMIT:$pfad" 2>/dev/null; then
  rot "Der Commit $aufloesung enthaelt kein Verzeichnis $pfad/ - falscher Stand?"
  exit 65
fi

tmp="$(mktemp -d)"
# Auch bei Strg-C wieder aufraeumen; ein liegengebliebenes Verzeichnis mit
# Manifesten ist Muell, den niemand zuordnen kann.
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "rollback: $PIPELINE auf $aufloesung"
echo "  Arbeitsbaum wird nicht angefasst - Manifeste kommen aus der Objektdatenbank."

git archive "$COMMIT" "$pfad" | tar -x -C "$tmp"

if [ ! -d "$tmp/$kustomize" ]; then
  rot "In $aufloesung fehlt $kustomize - dieser Stand laesst sich so nicht ausrollen."
  exit 65
fi

echo "  Rendern ..."
if ! kubectl kustomize "$tmp/$kustomize" > "$tmp/rendered.yaml"; then
  rot "Die Manifeste aus $aufloesung rendern nicht. Kein Rollback."
  exit 66
fi
echo "  $(grep -c '^kind:' "$tmp/rendered.yaml") Objekte gerendert."

if [ "$DRY" = "1" ]; then
  echo "  Probelauf (server-side dry-run), es wird nichts geaendert:"
  kubectl apply --dry-run=server -f "$tmp/rendered.yaml" | sed 's/^/    /'
  echo "rollback: Probelauf beendet, nichts angewendet."
  exit 0
fi

echo "  Anwenden auf $namespace ..."
kubectl apply -f "$tmp/rendered.yaml"

echo "  Nachpruefen ..."
kubectl rollout status -n "$namespace" --timeout=120s \
  $(kubectl get deploy,statefulset -n "$namespace" -o name 2>/dev/null | tr '\n' ' ')

cat <<ENDE

rollback: $PIPELINE steht auf $aufloesung.

Nicht vergessen - das leistet dieses Skript NICHT:

  * Datenbankmigrationen sind nicht zurueckgenommen. Laeuft der alte Stand gegen
    ein neueres Schema, kann er trotzdem scheitern.
  * Tickets, die der zurueckgerollte Deploy auf Fertig gezogen hat, muessen
    zurueck auf Test. Mission Control macht das; von Hand aufgerufen nicht.
ENDE
