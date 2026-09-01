#!/usr/bin/env sh
# Beantwortet die Frage aus HF-0086: passen unsere Limits und Requests zu dem,
# was die Pods wirklich verbrauchen - und wie viel Luft hat der Laptop noch?
#
# Die Zahlen in kubernetes/base/, monitoring/kubernetes/ und contracts.md
# Abschnitt 2 waren beim Schreiben der Manifeste geschaetzt und nie gegen
# Messwerte gehalten. Ohne dieses Skript ist der naechste Vergleich wieder eine
# Schaetzung.
#
# Liest ausschliesslich. Legt nichts an, aendert kein Manifest und schreibt
# nichts in den Cluster. Der Port-Forward auf Prometheus laeuft auf Loopback und
# wird am Ende wieder abgeraeumt.
set -eu

: "${CONTEXT:=kind-dev}"
: "${PROM_NS:=homeflow-monitoring}"
: "${PROM_SVC:=prometheus}"
# Prometheus haelt 7 Tage vor (contracts.md 2.2). Ein groesseres Fenster liefert
# keine aelteren Daten, sondern nur denselben Wert.
: "${WINDOW:=7d}"

usage() {
  echo "Aufruf: $0" >&2
  echo "  CONTEXT=$CONTEXT PROM_NS=$PROM_NS PROM_SVC=$PROM_SVC WINDOW=$WINDOW" >&2
  exit 64
}
[ $# -eq 0 ] || usage

command -v kubectl >/dev/null 2>&1 || { echo "kubectl fehlt" >&2; exit 69; }
command -v python3 >/dev/null 2>&1 || { echo "python3 fehlt" >&2; exit 69; }
kubectl config get-contexts "$CONTEXT" >/dev/null 2>&1 || {
  echo "Kontext '$CONTEXT' unbekannt" >&2; exit 69; }

# Freien Loopback-Port suchen, statt einen festen zu belegen - auf dem Laptop
# laeuft parallel oft schon ein `make port-forward`.
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')

PF_PID=""
cleanup() { [ -n "$PF_PID" ] && kill "$PF_PID" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

kubectl --context "$CONTEXT" -n "$PROM_NS" port-forward --address 127.0.0.1 \
  "svc/$PROM_SVC" "$PORT:9090" >/dev/null 2>&1 &
PF_PID=$!

i=0
while [ $i -lt 40 ]; do
  if curl -sf "http://127.0.0.1:$PORT/-/ready" >/dev/null 2>&1; then break; fi
  # Nicht pg_isready-Falle wiederholen: erst wenn /-/ready antwortet, ist der
  # Forward wirklich benutzbar.
  sleep 0.25
  i=$((i + 1))
done
curl -sf "http://127.0.0.1:$PORT/-/ready" >/dev/null 2>&1 || {
  echo "Prometheus ueber 127.0.0.1:$PORT nicht erreichbar" >&2; exit 69; }

PODS_JSON=$(kubectl --context "$CONTEXT" get pods -A -o json)
NODES_JSON=$(kubectl --context "$CONTEXT" get nodes -o json)
HOST_MEM=$(awk '/MemTotal/{print $2} /MemAvailable/{print $2}' /proc/meminfo | tr '\n' ' ')

export PORT WINDOW HOST_MEM
printf '%s' "$PODS_JSON" > /tmp/hf0086-pods.$$
printf '%s' "$NODES_JSON" > /tmp/hf0086-nodes.$$
trap 'cleanup; rm -f /tmp/hf0086-pods.'"$$"' /tmp/hf0086-nodes.'"$$" EXIT INT TERM

PODS_FILE=/tmp/hf0086-pods.$$ NODES_FILE=/tmp/hf0086-nodes.$$ python3 <<'PY'
import json, os, urllib.parse, urllib.request

port, window = os.environ["PORT"], os.environ["WINDOW"]
total_kb, avail_kb = (int(x) for x in os.environ["HOST_MEM"].split())
pods = json.load(open(os.environ["PODS_FILE"]))["items"]
nodes = json.load(open(os.environ["NODES_FILE"]))["items"]

MIB = 2 ** 20


def query(expr):
    url = "http://127.0.0.1:%s/api/v1/query?%s" % (
        port, urllib.parse.urlencode({"query": expr}))
    with urllib.request.urlopen(url, timeout=30) as r:
        body = json.load(r)
    if body.get("status") != "success":
        raise SystemExit("Prometheus-Abfrage fehlgeschlagen: %s" % body)
    return body["data"]["result"]


def series(expr):
    out = {}
    for s in query(expr):
        m = s["metric"]
        out[(m.get("namespace", "?"), m.get("container", "?"))] = float(s["value"][1])
    return out


def quantity(value, default=None):
    """Kubernetes-Mengen wie 192Mi oder 200m in Bytes bzw. mCPU."""
    if value is None:
        return default
    units = {"Ki": 2**10, "Mi": 2**20, "Gi": 2**30, "K": 10**3, "M": 10**6, "G": 10**9}
    for suffix, factor in units.items():
        if value.endswith(suffix):
            return float(value[:-len(suffix)]) * factor
    if value.endswith("m"):
        return float(value[:-1]) / 1000
    return float(value)


mem_peak = series(
    'max by (namespace,container) '
    '(max_over_time(container_memory_working_set_bytes{container!=""}[%s]))' % window)
cpu_peak = series(
    'max by (namespace,container) '
    '(max_over_time(rate(container_cpu_usage_seconds_total{container!=""}[5m])[%s:5m]))' % window)

configured, qos, killed = {}, {}, []
for p in pods:
    ns, name = p["metadata"]["namespace"], p["metadata"]["name"]
    qos[ns, name] = p["status"].get("qosClass", "?")
    for c in p["spec"]["containers"]:
        r = c.get("resources", {})
        configured[ns, c["name"]] = (
            quantity(r.get("requests", {}).get("memory")),
            quantity(r.get("limits", {}).get("memory")),
            quantity(r.get("limits", {}).get("cpu")),
            qos[ns, name],
        )
    for s in p["status"].get("containerStatuses", []):
        t = s.get("lastState", {}).get("terminated", {})
        if t.get("reason") == "OOMKilled":
            killed.append((ns, s["name"], t.get("finishedAt"), s["restartCount"]))

print("Speicher: Spitze im Fenster %s gegen das gesetzte Limit" % window)
print("  %-20s %-24s %>9s %>9s %>7s %>11s".replace(">", "")
      % ("namespace", "container", "Spitze", "Limit", "Anteil", "QoS"))
rows = []
for key, peak in mem_peak.items():
    req, lim, _, klass = configured.get(key, (None, None, None, "-"))
    rows.append((key[0], key[1], peak, lim, klass))
for ns, c, peak, lim, klass in sorted(rows, key=lambda r: -(r[2] / r[3] if r[3] else 0)):
    if lim:
        print("  %-20s %-24s %8.1fM %8.0fM %6.0f%% %11s"
              % (ns, c, peak / MIB, lim / MIB, peak / lim * 100, klass))
    else:
        print("  %-20s %-24s %8.1fM %9s %7s %11s"
              % (ns, c, peak / MIB, "-", "-", klass))
print("  Summe der Spitzen: %.0f MiB" % (sum(r[2] for r in rows) / MIB))

print()
print("CPU: Spitze im 5-Minuten-Mittel")
for key, peak in sorted(cpu_peak.items(), key=lambda kv: -kv[1])[:8]:
    _, _, lim, _ = configured.get(key, (None, None, None, None))
    print("  %-20s %-24s %6.0fm %s"
          % (key[0], key[1], peak * 1000,
             ("von %.0fm" % (lim * 1000)) if lim else "ohne Limit"))

print()
print("Knoten: was Kubernetes glaubt, gegen das, was der Laptop hat")
for n in nodes:
    st = n["status"]
    cap = quantity(st["capacity"]["memory"]) / MIB
    alloc = quantity(st["allocatable"]["memory"]) / MIB
    print("  %-20s capacity %.0f MiB, allocatable %.0f MiB, reserviert %.0f MiB"
          % (n["metadata"]["name"], cap, alloc, cap - alloc))
print("  Host tatsaechlich:   gesamt %.0f MiB, davon verfuegbar %.0f MiB"
      % (total_kb / 1024, avail_kb / 1024))
if avail_kb / 1024 < 1024:
    print("  ACHTUNG: unter 1 GiB frei - der Host-Kernel killt hier, nicht das Limit.")

print()
if killed:
    print("OOMKilled laut letztem Containerzustand:")
    for ns, c, when, restarts in sorted(killed, key=lambda k: k[2] or ""):
        print("  %-20s %-24s %s  (Neustarts: %d)" % (ns, c, when, restarts))
    print("  Fallen mehrere Zeitstempel in dieselbe Minute, war es Host-Druck,")
    print("  nicht ein einzelnes zu enges Limit.")
else:
    print("Kein Container meldet OOMKilled im letzten Zustand.")
PY
