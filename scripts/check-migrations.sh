#!/usr/bin/env sh
# Prüft backend/migrations/ auf die Fehler, die sonst erst in den Daten sichtbar
# werden.
#
# AGENTS.md nennt es unter den bekannten Fallen: "Migrationsnummern kollidieren
# still. Zwei parallel angelegte 000003_* fallen erst beim Merge auf."
# Abgesichert war das bisher nur durch die Reservierungsliste in
# docs/architecture/contracts.md 1.3 - also durch Disziplin zweier Assistenten,
# nicht durch eine Prüfung (HF-0098).
#
# Liest ausschließlich. Braucht keine Datenbank.
set -eu

cd "$(git rev-parse --show-toplevel)"
exec python3 - "$@" <<'PY'
import os, re, sys
from collections import defaultdict

MIG = "backend/migrations"
CONTRACTS = "docs/architecture/contracts.md"

fehler, warnungen = [], []

paare = defaultdict(dict)
namen = {}
for datei in sorted(os.listdir(MIG)):
    m = re.fullmatch(r"(\d{6})_([a-z0-9_]+)\.(up|down)\.sql", datei)
    if not m:
        if datei.endswith(".sql"):
            fehler.append(f"{datei}: Name folgt nicht <nummer>_<name>.<up|down>.sql")
        continue
    nummer, name, richtung = m.group(1), m.group(2), m.group(3)
    if richtung in paare[nummer] and paare[nummer][richtung] != name:
        fehler.append(
            f"Nummer {nummer} doppelt vergeben: "
            f"{paare[nummer][richtung]} und {name} - genau die stille Kollision "
            f"aus AGENTS.md")
    paare[nummer][richtung] = name
    namen.setdefault(nummer, set()).add(name)

for nummer, seiten in sorted(paare.items()):
    if len(namen[nummer]) > 1:
        fehler.append(f"Nummer {nummer} trägt mehrere Namen: {sorted(namen[nummer])}")
    if "up" not in seiten:
        fehler.append(f"{nummer}_{seiten.get('down')}: .down.sql ohne .up.sql")
    if "down" not in seiten:
        # Ohne down ist kein Rollback möglich (HF-0104 hängt daran).
        fehler.append(f"{nummer}_{seiten.get('up')}: .up.sql ohne .down.sql")

nummern = sorted(int(n) for n in paare)
if nummern:
    fehlend = [n for n in range(1, max(nummern) + 1) if n not in nummern]
    if fehlend:
        # Warnung, kein Fehler: eine Lücke kann gewollt sein, ist aber fast immer
        # eine gelöschte Migration, die anderswo schon lief.
        warnungen.append("Lücke in der Nummernfolge: " +
                         ", ".join(f"{n:06d}" for n in fehlend))

try:
    text = open(CONTRACTS, encoding="utf-8").read()
except FileNotFoundError:
    fehler.append(f"{CONTRACTS} fehlt - die Reservierungsliste ist die einzige "
                  "Absprache zwischen den Assistenten")
    text = ""

reserviert = set(re.findall(r"`(\d{6})_[a-z0-9_]+`", text))
# 000001 und 000002 stehen im Fließtext, nicht als Reservierung.
vorgabe = {"000001", "000002"}
nicht_reserviert = sorted(set(paare) - reserviert - vorgabe)
if nicht_reserviert:
    fehler.append(
        "Nicht in contracts.md 1.3 reserviert: " + ", ".join(nicht_reserviert) +
        " - Nummer eintragen oder die Migration umbenennen")

reserviert_ohne_datei = sorted(reserviert - set(paare))
if reserviert_ohne_datei:
    # Kein Fehler: eine Reservierung ohne Datei ist der vorgesehene Zustand,
    # solange jemand die Migration noch schreibt.
    warnungen.append("Reserviert, aber noch keine Datei: " +
                     ", ".join(reserviert_ohne_datei))

for w in warnungen:
    print(f"  WARNUNG  {w}")
for f in fehler:
    print(f"  FEHLER   {f}")
print(f"  {len(paare)} Migrationen, {len(reserviert)} Reservierungen, "
      f"{len(fehler)} Fehler, {len(warnungen)} Warnungen")
sys.exit(1 if fehler else 0)
PY
