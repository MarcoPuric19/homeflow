#!/usr/bin/env sh
# Prüft die Alarmregeln - nicht nur ihre Syntax, sondern ihre Wirkung.
#
# Anlass ist ein eigener Fehler aus HF-0092: HomeflowClusterMemoryCommitted war
# syntaktisch einwandfrei, hat gerendert und ausgewertet, und hätte das
# Ereignis, für das sie geschrieben war, nie gemeldet. Aufgefallen ist das nur,
# weil ich den Ausdruck zufällig gegen die aufgezeichnete Historie gehalten
# habe. `kubectl kustomize` beantwortet die Frage nicht (HF-0103).
#
# promtool ist auf diesem Laptop nicht installiert und soll es auch nicht sein.
# Es steckt im Prometheus-Image, das für den Cluster ohnehin lokal liegt - ein
# Wegwerf-Container kostet nichts Dauerhaftes.
#
# Liest ausschließlich.
set -eu

: "${PROM_IMAGE:=prom/prometheus:v3.13.0-distroless}"
: "${TESTS:=monitoring/alert-tests.yaml}"

[ $# -eq 0 ] || { echo "Aufruf: $0" >&2; exit 64; }
cd "$(git rev-parse --show-toplevel)"
for werkzeug in docker kubectl python3; do
  command -v "$werkzeug" >/dev/null 2>&1 || { echo "$werkzeug fehlt" >&2; exit 69; }
done

AUS=$(mktemp -d)
chmod 0777 "$AUS"
trap 'rm -rf "$AUS"' EXIT INT TERM

# Die Regeln stehen als ConfigMap-Schlüssel im Manifest. Sie von dort zu holen
# statt aus einer zweiten Datei ist der Punkt: geprüft wird, was ausgerollt
# wird, und nicht eine Kopie, die auseinanderlaufen kann.
kubectl kustomize monitoring/kubernetes | python3 -c "
import sys, yaml
for doc in yaml.safe_load_all(sys.stdin):
    if doc and doc.get('kind') == 'ConfigMap' and 'alerts.yml' in (doc.get('data') or {}):
        sys.stdout.write(doc['data']['alerts.yml'])
        break
else:
    sys.exit('alerts.yml in keinem ConfigMap gefunden')
" > "$AUS/rules.yml"

if [ ! -s "$AUS/rules.yml" ]; then
  echo "FEHLER  Keine Regeln aus monitoring/kubernetes gerendert." >&2
  exit 1
fi
cp "$TESTS" "$AUS/tests.yaml"

lauf() {
  docker run --rm --network none -v "$AUS":/work:ro -w /work \
    --entrypoint promtool "$PROM_IMAGE" "$@"
}

echo "  Syntax:"
lauf check rules rules.yml | sed 's/^/    /'
echo "  Wirkung:"
lauf test rules tests.yaml | sed 's/^/    /'
