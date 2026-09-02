#!/usr/bin/env sh
# Prüft nach dem Anwenden, ob wirklich ausgeliefert wurde - und erzwingt es,
# wenn nicht.
#
# Anlass ist HF-0135: ein Lauf am 2026-09-02 hat Erfolg gemeldet und nichts
# ausgeliefert. VERSION stand unverändert, also war der Deployment-Spec nach
# `kubectl apply` derselbe, Kubernetes sah keine Änderung und rollte nicht aus.
# Mit `imagePullPolicy: IfNotPresent` behielt der Pod sein 15 Stunden altes
# Image. Alle vier Schritte einzeln erfolgreich, das Ergebnis trotzdem nichts.
#
# Ein Lauf, der nichts tut, darf nicht grün sein. Verglichen werden deshalb
# Image-IDs und nicht Zeitstempel: derselbe Tag kann drei verschiedene Inhalte
# meinen, und genau das war der Fall.
set -eu

: "${CONTEXT:=kind-dev}"
: "${NAMESPACE:=homeflow}"
: "${NODE:=dev-control-plane}"
: "${DEPLOYMENT:=homeflow-api}"
: "${SELECTOR:=app.kubernetes.io/name=homeflow-api}"
: "${ROLLOUT_TIMEOUT:=180s}"

NUR_PRUEFEN=0
case "${1:-}" in
  "")        ;;
  --pruefen) NUR_PRUEFEN=1 ;;
  *)
    echo "Aufruf: $0 [--pruefen]" >&2
    echo "  --pruefen  meldet nur, ob ausgeliefert wurde, und ändert nichts" >&2
    exit 64 ;;
esac
for werkzeug in kubectl docker python3; do
  command -v "$werkzeug" >/dev/null 2>&1 || { echo "$werkzeug fehlt" >&2; exit 69; }
done

kubectl_() { kubectl --context "$CONTEXT" -n "$NAMESPACE" "$@"; }

tag=$(kubectl_ get deployment "$DEPLOYMENT" -o jsonpath='{.spec.template.spec.containers[0].image}')
[ -n "$tag" ] || { echo "Kein Image im Deployment $DEPLOYMENT" >&2; exit 69; }

# Was liegt im Node unter diesem Tag? Das ist der Stand, den ein neuer Pod
# bekommen würde.
node_image() {
  docker exec "$NODE" crictl images -o json 2>/dev/null | python3 -c "
import json,sys
tag=sys.argv[1]
try:
    d=json.load(sys.stdin)
except Exception:
    sys.exit(0)
for image in d.get('images',[]):
    if tag in image.get('repoTags',[]):
        print(image['id'])
        break
" "$tag"
}

# Worauf laufen die Pods, die tatsächlich Verkehr bedienen?
#
# Nur bereite Pods zählen, und beendende gar nicht. Die erste Fassung sammelte
# alle ein - während eines Rollouts sind das zwei, der neue und der alte, und
# der Vergleich lief gegen zwei Zeilen statt gegen eine. Ergebnis war ein
# Fehlalarm: Lauf #2 am 2026-09-02 hat korrekt ausgeliefert und galt trotzdem
# als fehlgeschlagen.
#
# Ein crashloopender Pod ist nicht bereit und faellt damit heraus - genau
# richtig, denn er bedient niemanden.
pod_images() {
  kubectl_ get pods -l "$SELECTOR" -o json 2>/dev/null | python3 -c "
import json,sys
try:
    data=json.load(sys.stdin)
except Exception:
    sys.exit(0)
for pod in data.get('items',[]):
    if pod['metadata'].get('deletionTimestamp'):
        continue
    for status in pod['status'].get('containerStatuses',[]):
        if not status.get('ready'):
            continue
        image=status.get('imageID','')
        if '@' in image:
            image=image.split('@',1)[1]
        print(image.removeprefix('sha256:'))
" | sort -u | grep -v '^$' || true
}

soll=$(node_image)
if [ -z "$soll" ]; then
  echo "WARNUNG  Tag $tag ist im Node $NODE nicht auffindbar - Prüfung übersprungen." >&2
  exit 0
fi
soll=${soll#sha256:}

ist=$(pod_images)
printf "  Image laut Node:  %.12s\n" "$soll"
if [ -z "$ist" ]; then
  echo "  Kein bereiter Pod - nichts, was Verkehr bedienen könnte."
else
  printf "  Bereite Pods auf: %s\n" "$(echo "$ist" | cut -c1-12 | tr '\n' ' ')"
fi

if [ -n "$ist" ] && [ "$ist" = "$soll" ]; then
  echo "  Ausgeliefert: der bereite Pod nutzt das Image aus dem Node."
  exit 0
fi

echo "  Abweichung: der Pod läuft auf einem anderen Image als im Node liegt."
if [ "$NUR_PRUEFEN" -eq 1 ]; then
  echo "  (--pruefen: nichts geändert)"
  exit 1
fi
# Sonst ausliefern statt nur melden - es wurde ein Deploy angefordert, kein
# Bericht.
echo "  Erzwinge Rollout (gleicher Tag, anderer Inhalt - deshalb sieht kubectl apply nichts)."
kubectl_ rollout restart "deployment/$DEPLOYMENT" >/dev/null

# Ein Pod, der nach der Migration abbricht, wird hier nicht bereit. Genau dann
# muss der Lauf rot werden statt durchzurutschen.
if ! kubectl_ rollout status "deployment/$DEPLOYMENT" --timeout="$ROLLOUT_TIMEOUT"; then
  echo "FEHLER  Der neue Pod wurde nicht bereit. Häufigster Grund: die Migration" >&2
  echo "        bricht ab. Letzte Zeilen:" >&2
  kubectl_ logs "deployment/$DEPLOYMENT" --tail=20 2>&1 | sed 's/^/        /' >&2
  exit 1
fi

ist=$(pod_images)
if [ -z "$ist" ] || [ "$ist" != "$soll" ]; then
  echo "FEHLER  Auch nach dem Rollout bedient kein Pod mit dem Image aus dem Node." >&2
  printf "        Node:   %s\n" "$soll" >&2
  printf "        bereit: %s\n" "$(echo "${ist:-keiner}" | tr '\n' ' ')" >&2
  exit 1
fi
echo "  Ausgeliefert nach erzwungenem Rollout."
