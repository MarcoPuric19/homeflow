#!/usr/bin/env sh
# Hält docs/architecture/contracts.md Abschnitt 2.1 gegen die tatsächlich
# gerenderten Manifeste - und die Summe aller Requests gegen das, was der Knoten
# nach den Reservierungen aus kubernetes/kind-cluster.yaml überhaupt vergeben
# kann.
#
# Am 2026-09-01 stimmte die Tabelle, weil ich sie von Hand abgeglichen habe.
# Genau das gehört ins Repository statt in eine Sitzung (HF-0100).
#
# Der Abgleich läuft nicht über Anzeigenamen, sondern über die Quelldatei in der
# letzten Tabellenspalte. Damit pflegt sich die Zuordnung selbst: wer eine
# Komponente umbenennt, muss nichts an diesem Skript ändern.
#
# Liest ausschließlich und braucht keinen Cluster - kubectl kustomize rendert
# offline.
set -eu

cd "$(git rev-parse --show-toplevel)"
command -v kubectl >/dev/null 2>&1 || { echo "kubectl fehlt" >&2; exit 69; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM
kubectl kustomize kubernetes/base > "$TMP/base.yaml"
kubectl kustomize monitoring/kubernetes > "$TMP/monitoring.yaml"

RENDER_DIR="$TMP" exec python3 - <<'PY'
import os, re, sys, yaml

MIB = 2 ** 20
CONTRACTS = "docs/architecture/contracts.md"
KIND = "kubernetes/kind-cluster.yaml"
fehler = []


def menge(v):
    """Kubernetes-Menge -> Bytes bzw. mCPU. None bleibt None."""
    if v is None:
        return None
    for suffix, faktor in (("Ki", 2**10), ("Mi", 2**20), ("Gi", 2**30)):
        if v.endswith(suffix):
            return int(float(v[:-2]) * faktor)
    if v.endswith("m"):
        return int(v[:-1])
    return int(float(v))


# --- Ist-Zustand aus den gerenderten Manifesten -----------------------------
ist = {}          # quelldatei -> (req_mem, req_cpu, lim_mem, lim_cpu)
quelle_von = {}   # containername -> quelldatei
for name in ("base.yaml", "monitoring.yaml"):
    for doc in yaml.safe_load_all(open(os.path.join(os.environ["RENDER_DIR"], name))):
        if not doc or doc.get("kind") not in ("Deployment", "StatefulSet"):
            continue
        for c in doc["spec"]["template"]["spec"]["containers"]:
            r = c.get("resources", {})
            ist[c["name"]] = (
                menge(r.get("requests", {}).get("memory")),
                menge(r.get("requests", {}).get("cpu")),
                menge(r.get("limits", {}).get("memory")),
                menge(r.get("limits", {}).get("cpu")),
            )

# Container einer Manifestdatei zuordnen, damit die Tabelle über ihre
# Quellenspalte auflösbar ist.
for pfad in ("kubernetes/base", "monitoring/kubernetes"):
    for datei in sorted(os.listdir(pfad)):
        if not datei.endswith(".yaml"):
            continue
        voll = os.path.join(pfad, datei)
        try:
            docs = list(yaml.safe_load_all(open(voll, encoding="utf-8")))
        except yaml.YAMLError:
            continue
        for doc in docs:
            if not doc or doc.get("kind") not in ("Deployment", "StatefulSet"):
                continue
            for c in doc["spec"]["template"]["spec"]["containers"]:
                quelle_von[voll] = c["name"]

# --- Soll-Zustand aus der Tabelle in contracts.md 2.1 -----------------------
text = open(CONTRACTS, encoding="utf-8").read()
abschnitt = text.split("### 2.1")[1].split("### 2.2")[0]
zeile = re.compile(
    r"^\|\s*(?P<name>[^|]+?)\s*\|\s*(?P<rm>[\d.]+)\s*MiB\s*/\s*(?P<rc>\d+)\s*mCPU\s*"
    r"\|\s*(?P<lm>[\d.]+)\s*MiB\s*/\s*(?P<lc>\d+)\s*mCPU\s*\|[^|]*\|(?P<quelle>[^|]*)\|",
    re.M)

geprueft = 0
gesamt_row = None
for m in zeile.finditer(abschnitt.replace("**", "")):
    name = m.group("name").strip()
    if name.lower().startswith("komponente") or set(name) <= set("- "):
        continue
    soll = (int(float(m.group("rm"))) * MIB, int(m.group("rc")),
            int(float(m.group("lm"))) * MIB, int(m.group("lc")))
    if "gesamt" in name.lower():
        gesamt_row = soll
        continue
    pfade = re.findall(r"`((?:kubernetes|monitoring)/[^`]+\.yaml)`", m.group("quelle"))
    if not pfade:
        fehler.append(f"Zeile '{name}': keine Manifestdatei in der Quellenspalte")
        continue
    container = quelle_von.get(pfade[0])
    if container is None:
        fehler.append(f"Zeile '{name}': {pfade[0]} enthält kein Deployment/StatefulSet")
        continue
    if container not in ist:
        fehler.append(f"Zeile '{name}': Container '{container}' nicht im Rendering")
        continue
    geprueft += 1
    for i, was in enumerate(("Request-Speicher", "Request-CPU", "Limit-Speicher", "Limit-CPU")):
        if ist[container][i] != soll[i]:
            einheit = "MiB" if "Speicher" in was else "mCPU"
            teiler = MIB if "Speicher" in was else 1
            fehler.append(
                f"{container}: {was} steht im Manifest auf "
                f"{ist[container][i] // teiler} {einheit}, in contracts.md 2.1 auf "
                f"{soll[i] // teiler} {einheit}")

if geprueft == 0:
    fehler.append("Keine Tabellenzeile aufgelöst - Tabellenformat in 2.1 geändert?")

# --- Summenzeile Monitoring ------------------------------------------------
mon = [c for p, c in quelle_von.items() if p.startswith("monitoring/")]
if gesamt_row and mon:
    for i, was in enumerate(("Request-Speicher", "Request-CPU", "Limit-Speicher", "Limit-CPU")):
        summe = sum(ist[c][i] or 0 for c in mon)
        if summe != gesamt_row[i]:
            teiler = MIB if "Speicher" in was else 1
            fehler.append(f"Monitoring-Summe {was}: gerendert {summe // teiler}, "
                          f"contracts.md {gesamt_row[i] // teiler}")

# --- Die wichtigere Richtung: passt die Summe überhaupt auf den Knoten? -----
reserviert = 0
try:
    kind = yaml.safe_load(open(KIND, encoding="utf-8"))
    patch = yaml.safe_load(kind["nodes"][0]["kubeadmConfigPatches"][0])
    for feld in ("systemReserved", "kubeReserved"):
        reserviert += menge(patch.get(feld, {}).get("memory", "0"))
    reserviert += menge(patch.get("evictionHard", {}).get("memory.available", "0"))
except (FileNotFoundError, KeyError, IndexError, yaml.YAMLError) as e:
    fehler.append(f"{KIND} nicht auswertbar ({e}) - ohne sie ist die "
                  "allocatable-Prüfung blind")

hostkb = 0
for line in open("/proc/meminfo"):
    if line.startswith("MemTotal"):
        hostkb = int(line.split()[1])
        break

summe_req = sum(v[0] or 0 for v in ist.values())
print(f"  {geprueft} Tabellenzeilen gegen die Manifeste geprüft")
if hostkb and reserviert:
    allocatable = hostkb * 1024 - reserviert
    anteil = summe_req / allocatable * 100
    print(f"  Requests gesamt {summe_req // MIB} MiB gegen allocatable "
          f"{allocatable // MIB} MiB ({anteil:.0f} %)")
    if summe_req > allocatable:
        fehler.append(
            f"Die Summe der Requests ({summe_req // MIB} MiB) übersteigt "
            f"allocatable ({allocatable // MIB} MiB). Der Cluster ließe sich so "
            "nicht mehr vollständig planen.")
    elif anteil > 80:
        print(f"  WARNUNG  über 80 % von allocatable belegt")

for f in fehler:
    print(f"  FEHLER   {f}")
sys.exit(1 if fehler else 0)
PY
