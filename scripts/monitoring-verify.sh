#!/usr/bin/env sh
# Fragt nach dem Anwenden, ob der Monitoring-Stack wirklich läuft.
#
# Entstanden aus MC-0019: Mission Control schloss von Exit-Code 0 auf
# "ausgeliefert". Der Schutz dagegen verlangt von jedem Ablauf, der den Cluster
# verändert, eine Nachprüfung - und deckte damit auf, dass auch das Monitoring
# keine hatte. `kubectl apply` sagt nur, dass die Objekte angenommen wurden,
# nicht dass etwas davon läuft.
#
# Bewusst über die Bereitschaft der Arbeitslasten und nicht über `kubectl diff`:
# ein Diff meldet auch server-seitige Vorgaben und wäre eine Quelle falscher
# roter Läufe. "Läuft es?" ist die Frage, die zählt.
set -eu

: "${CONTEXT:=kind-dev}"
: "${NAMESPACE:=homeflow-monitoring}"
: "${ROLLOUT_TIMEOUT:=120s}"

[ $# -eq 0 ] || { echo "Aufruf: $0" >&2; exit 64; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl fehlt" >&2; exit 69; }

kubectl_() { kubectl --context "$CONTEXT" -n "$NAMESPACE" "$@"; }

arbeitslasten=$(kubectl_ get deployments,statefulsets -o name 2>/dev/null || true)
if [ -z "$arbeitslasten" ]; then
  echo "FEHLER  Im Namespace $NAMESPACE läuft keine Arbeitslast." >&2
  exit 1
fi

fehler=0
for objekt in $arbeitslasten; do
  if kubectl_ rollout status "$objekt" --timeout="$ROLLOUT_TIMEOUT" >/dev/null 2>&1; then
    echo "  bereit    $objekt"
  else
    echo "  NICHT BEREIT $objekt" >&2
    kubectl_ get "$objekt" -o wide 2>&1 | tail -1 | sed 's/^/          /' >&2
    fehler=1
  fi
done

if [ "$fehler" -ne 0 ]; then
  echo "FEHLER  Angewendet ist nicht dasselbe wie ausgeliefert." >&2
  exit 1
fi
