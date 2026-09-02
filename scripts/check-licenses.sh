#!/usr/bin/env sh
# Prüft, dass jede direkte Abhängigkeit in docs/legal/dependencies.md erfasst ist.
#
# Eine Lizenzübersicht, die einmal geschrieben und dann nie wieder angefasst
# wird, ist binnen eines Monats falsch - und zwar unbemerkt, weil nichts
# fehlschlägt. Genau dieselbe Fehlerart wie bei den Ressourcenkontrakten
# (HF-0100) und den Migrationsnummern (HF-0098): still, folgenlos, bis es
# jemanden interessiert.
#
# Das Skript beantwortet NICHT, ob eine Lizenz in Ordnung ist. Das kann kein
# Werkzeug: ob ein AGPL-Bestandteil in einer bestimmten Betriebsart tragbar
# ist, ist eine Abwägung, keine Abfrage. Geprüft wird nur, dass die
# Abhängigkeit überhaupt *bewertet wurde*.

set -eu

wurzel="$(cd "$(dirname "$0")/.." && pwd)"
uebersicht="$wurzel/docs/legal/dependencies.md"

if [ ! -f "$uebersicht" ]; then
  echo "check-licenses: $uebersicht fehlt" >&2
  exit 1
fi

python3 - "$wurzel" "$uebersicht" <<'PY'
import json, pathlib, re, sys

wurzel = pathlib.Path(sys.argv[1])
text = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8")

fehlend = []
geprueft = 0


def erfasst(name):
    return name in text


# --- Go: direkte requires aus allen Modulen ------------------------------
for modul in ("backend", "integrations", "missioncontrol"):
    gomod = wurzel / modul / "go.mod"
    if not gomod.exists():
        continue
    inhalt = gomod.read_text(encoding="utf-8")
    # Direkte UND transitive requires. Transitive landen mit im Binary,
    # also werden sie auch erfasst - der erste Entwurf filterte sie heraus
    # und der Filter griff nicht einmal richtig.
    for treffer in re.finditer(r"^\s*(\S+)\s+v\S+", inhalt, re.M):
        pfad = treffer.group(1)
        if pfad in ("module", "go", "toolchain", "require", "replace", "exclude"):
            continue
        if "/" not in pfad:
            continue
        geprueft += 1
        if not erfasst(pfad):
            fehlend.append(f"Go-Modul {pfad} ({modul}/go.mod)")

# --- npm: dependencies und devDependencies ------------------------------
pkg = wurzel / "frontend" / "package.json"
if pkg.exists():
    d = json.loads(pkg.read_text(encoding="utf-8"))
    for art in ("dependencies", "devDependencies"):
        for name in d.get(art, {}):
            geprueft += 1
            if not erfasst(name):
                fehlend.append(f"npm-Paket {name} (frontend/package.json)")

# --- Container-Images aus den Manifesten ---------------------------------
for verzeichnis in ("kubernetes", "monitoring"):
    basis = wurzel / verzeichnis
    if not basis.is_dir():
        continue
    for datei in basis.rglob("*.yaml"):
        for treffer in re.finditer(r"^\s*image:\s*(\S+)", datei.read_text(encoding="utf-8"), re.M):
            bild = treffer.group(1).strip('"\'')
            # Das eigene Image ist keine Fremdabhängigkeit.
            if "homeflow" in bild:
                continue
            name = bild.split(":")[0]
            geprueft += 1
            if not erfasst(name):
                fehlend.append(f"Image {bild} ({datei.relative_to(wurzel)})")

if fehlend:
    print("check-licenses: nicht in docs/legal/dependencies.md erfasst:", file=sys.stderr)
    for f in sorted(set(fehlend)):
        print(f"  {f}", file=sys.stderr)
    print("", file=sys.stderr)
    print("Jede neue Abhängigkeit braucht dort eine Zeile mit ihrer Lizenz -", file=sys.stderr)
    print("und bei Copyleft einen Satz dazu, warum sie hier tragbar ist.", file=sys.stderr)
    print("Im Zweifel nicht einbauen, sondern als Beobachtung anlegen und fragen.", file=sys.stderr)
    sys.exit(1)

print(f"check-licenses: {geprueft} Abhängigkeiten geprüft, alle erfasst")
PY
