#!/usr/bin/env bash
# Erzeugt aus diesem Repository einen abgeleiteten, veröffentlichungsfähigen
# Klon - und fasst das Original dabei nicht an.
#
# Warum überhaupt ein Rewrite: GitHub zeigt die vollständige Historie. Ein
# `git rm` heute entfernt eine Datei aus der Gegenwart, nicht aus den 27
# Commits davor. Wer Anweisungen "verstecken" will, indem er sie löscht und
# committet, veröffentlicht sie trotzdem - nur eine Version jünger.
#
# Das Skript pusht NICHT. Es baut den Klon, prüft ihn und sagt, was zu tun
# wäre. Ein Skript, das ungefragt veröffentlicht, ist genau einen Bedienfehler
# von einem Datenleck entfernt (HF-0140).

set -euo pipefail

# ---------------------------------------------------------------------------
# Was aus JEDER Version der Historie verschwindet.
#
# Diese Liste ist die eigentliche Entscheidung. Sie steht deshalb oben und
# nicht irgendwo im Ablauf. Marco hat am 2026-09-02 festgelegt: die drei
# Assistenten-Anweisungen fliegen, die Tickets bleiben.
# ---------------------------------------------------------------------------
AUSSCHLUSS=(
  "AGENTS.md"
  "CLAUDE.md"
  "Austausch.md"
  # "missioncontrol/tickets"   # <- eine Zeile, falls die Tickets doch weg sollen
)

# Trailer, die aus jeder Commit-Nachricht entfernt werden.
# Woertliche Ersetzungen ueber die gesamte Historie. Gitignoriert, weil die
# Datei die zu schwaerzenden Werte im Klartext enthaelt. Fehlt sie, wird nichts
# ersetzt - und das Skript sagt es deutlich, statt es zu verschweigen.
SCHWAERZUNGEN="$(cd "$(dirname "$0")/.." && pwd)/redactions.local.txt"

TRAILER_MUSTER='^[[:space:]]*Co-[Aa]uthored-[Bb]y:.*$'

QUELLE="$(cd "$(dirname "$0")/.." && pwd)"
ZIEL="${1:-${TMPDIR:-/tmp}/homeflow-public}"

rot()  { printf '\033[31m%s\033[0m\n' "$*" >&2; }
info() { printf '  %s\n' "$*"; }

# --- Voraussetzungen -------------------------------------------------------

if ! git filter-repo --version >/dev/null 2>&1 && ! command -v git-filter-repo >/dev/null 2>&1; then
  rot "git-filter-repo fehlt."
  cat >&2 <<'HILFE'

  Es ist ein einzelnes Python-Skript unter MIT-Lizenz, kostenlos und ohne
  Konto - aber die Installation ist deine Entscheidung, nicht meine:

      sudo apt install git-filter-repo        # Debian/Ubuntu
      pipx install git-filter-repo            # ohne Systempaket

  Warum nicht git filter-branch: es ist von Git selbst als fehleranfällig
  zurückgezogen und würde bei 50 Commits mehrere Minuten brauchen, ohne die
  Nachprüfung zu ersparen.
HILFE
  exit 1
fi

if [ -e "$ZIEL" ]; then
  rot "Zielverzeichnis existiert bereits: $ZIEL"
  rot "Bitte löschen oder einen anderen Pfad angeben - ich räume nichts weg, das ich nicht angelegt habe."
  exit 1
fi

nicht_committet="$(cd "$QUELLE" && git status --porcelain | wc -l)"

echo "Export nach $ZIEL"
echo

# --- Klonen ----------------------------------------------------------------
# --no-local erzwingt echte Objektübertragung statt Hardlinks. Ohne das kann
# ein Rewrite im Klon auf Objekte der Quelle durchschlagen.

info "Klone (ohne Hardlinks) ..."
git clone --no-local --quiet "$QUELLE" "$ZIEL"
cd "$ZIEL"
git remote remove origin 2>/dev/null || true

vorher="$(git rev-list --count HEAD)"

# --- Filtern ---------------------------------------------------------------

pfad_argumente=()
for p in "${AUSSCHLUSS[@]}"; do
  pfad_argumente+=(--path "$p")
done

info "Entferne ${#AUSSCHLUSS[@]} Pfad(e) aus jeder Version ..."
git filter-repo --quiet --force --invert-paths "${pfad_argumente[@]}"

info "Entferne Co-Authored-By-Trailer aus jeder Commit-Nachricht ..."
git filter-repo --quiet --force --message-callback '
import re
return re.sub(rb"(?im)^[ \t]*co-authored-by:.*(?:\r?\n)?", b"", message)
'

if [ -f "$SCHWAERZUNGEN" ]; then
  # filter-repo kennt keine Kommentare im Ersetzungsformat - also vorher raus.
  regeln="$(mktemp)"
  grep -vE '^[[:space:]]*(#|$)' "$SCHWAERZUNGEN" > "$regeln"
  anzahl="$(wc -l < "$regeln" | tr -d ' ')"
  if [ "$anzahl" -gt 0 ]; then
    info "Wende $anzahl woertliche Schwaerzung(en) auf die Historie an ..."
    git filter-repo --quiet --force --replace-text "$regeln"
  fi
  rm -f "$regeln"
else
  rot "Hinweis: $SCHWAERZUNGEN fehlt - es wird kein Textersatz vorgenommen."
  rot "Die Historie geht so raus, wie sie ist. Das kann richtig sein; gewusst haben musst du es."
fi

nachher="$(git rev-list --count HEAD)"

# --- Nachprüfen: behaupten reicht nicht ------------------------------------
#
# Ab hier gilt dieselbe Regel wie überall: grün heißt nicht geändert. Jeder
# ausgeschlossene Pfad und jeder Trailer wird gegen die GESAMTE neue Historie
# geprüft, nicht gegen den Arbeitsbaum.

echo
echo "Nachprüfung gegen die vollständige neue Historie:"
fehler=0

alle_dateien="$(git log --all --pretty=format: --name-only | sort -u | sed '/^$/d')"

for p in "${AUSSCHLUSS[@]}"; do
  if printf '%s\n' "$alle_dateien" | grep -qx -e "$p" || printf '%s\n' "$alle_dateien" | grep -q "^$p/"; then
    rot "  NOCH VORHANDEN in der Historie: $p"
    fehler=1
  else
    info "sauber: $p"
  fi
done

if [ -f "$SCHWAERZUNGEN" ]; then
  grep -vE '^[[:space:]]*(#|$)' "$SCHWAERZUNGEN" | while IFS= read -r zeile; do
    original="${zeile%%==>*}"
    [ -z "$original" ] && continue
    if git log --all -p 2>/dev/null | grep -qF -- "$original"; then
      rot "  UEBERLEBT die Schwaerzung: $original"
      echo x >> "$ZIEL/.schwaerzung-fehler"
    fi
  done
  if [ -f "$ZIEL/.schwaerzung-fehler" ]; then
    fehler=1
    rm -f "$ZIEL/.schwaerzung-fehler"
  else
    info "sauber: alle Schwaerzungen greifen in der gesamten Historie"
  fi
fi

trailer_rest="$(git log --all --pretty=format:%B | grep -cE "$TRAILER_MUSTER" || true)"
if [ "$trailer_rest" -gt 0 ]; then
  rot "  NOCH VORHANDEN: $trailer_rest Co-Authored-By-Trailer"
  fehler=1
else
  info "sauber: keine Co-Authored-By-Trailer"
fi

if [ "$fehler" -ne 0 ]; then
  echo
  rot "Export NICHT freigegeben. Der Klon bleibt zur Ansicht liegen, wird aber nicht gepusht."
  exit 2
fi

# --- Personenbezogene Daten und Geheimnisse --------------------------------
#
# Die eigentliche rechtliche Gefahr beim Veröffentlichen ist nicht NIS2 oder
# der CRA - beide greifen hier nicht (docs/legal/compliance.md). Sie ist, dass
# personenbezogene Daten mit rausgehen. Die DSGVO-Haushaltsausnahme trägt genau
# so lange, wie nichts davon veröffentlicht wird.
#
# Gesucht wird in der VOLLSTÄNDIGEN Historie, nicht im Arbeitsbaum. Was in
# Commit 3 stand und in Commit 4 gelöscht wurde, ist auf GitHub weiterhin
# lesbar.

echo
echo "Suche nach personenbezogenen Daten und Geheimnissen:"

verlauf="$(git log --all -p 2>/dev/null || true)"

treffer_zahl() { printf '%s' "$verlauf" | grep -cE "$1" || true; }

# Harte Sperren. Nichts davon gehört jemals in ein Repository, und ein Fund
# ist kein Abwägungsfall.
for muster in 'BEGIN [A-Z ]*PRIVATE KEY' '\bDE[0-9]{20}\b' '\b[0-9]{16}\b'; do
  n="$(treffer_zahl "$muster")"
  if [ "$n" -gt 0 ]; then
    rot "  SPERRE: $n Treffer für /$muster/ in der Historie"
    fehler=1
  fi
done
[ "$fehler" -eq 0 ] && info "keine Schlüssel, IBANs oder Kartennummern"

# E-Mail-Adressen: kein Abbruch, aber sichtbar. Die eigene Adresse in den
# Commit-Metadaten ist erwartbar und die Entscheidung des Autors; eine fremde
# ist es nicht.
adressen="$(printf '%s' "$verlauf" \
  | grep -ohE '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-z]{2,}' \
  | grep -viE 'example|noreply|localhost|\.local|@AGENTS|@CLAUDE' \
  | sort | uniq -c | sort -rn || true)"

if [ -n "$adressen" ]; then
  echo "  E-Mail-Adressen in der Historie (Anzahl, Adresse):"
  printf '%s\n' "$adressen" | sed 's/^/    /'
  echo "    Eigene Adressen sind erwartbar - fremde nicht. Bitte durchsehen."
else
  info "keine E-Mail-Adressen"
fi

if [ "$fehler" -ne 0 ]; then
  echo
  rot "Export NICHT freigegeben: die Historie enthält Daten, die nicht nach draußen dürfen."
  exit 3
fi

# --- Was bleibt sichtbar ---------------------------------------------------
#
# Der unbequeme Teil. Ein Export, der nur meldet, was er entfernt hat,
# erzeugt ein falsches Sicherheitsgefühl.

echo
echo "Was nach dem Export weiterhin öffentlich lesbar ist:"

zaehle() { git grep -I -c "$1" -- "${2:-.}" 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'; }

info "$(zaehle 'Claude Code') Fundstellen 'Claude Code' im Arbeitsbaum"
info "$(zaehle 'Codex') Fundstellen 'Codex'"
info "$(zaehle 'Marco') Fundstellen 'Marco'"
info "$(git log --oneline | grep -cE '\(Codex\)' || true) Commit-Betreffs mit '(Codex)'"
info "$(git log --oneline | grep -cE 'HF-[0-9]{4}|MC-[0-9]{4}' || true) Commit-Betreffs mit Ticketbezug"

echo
echo "Historie: $vorher Commits vorher, $nachher nachher."
echo "Größe:    $(du -sh .git | cut -f1)"
if [ "$nicht_committet" -gt 0 ]; then
  echo
  rot "Achtung: in der Quelle liegen $nicht_committet nicht committete Änderungen."
  rot "Was nicht committet ist, steckt auch nicht in diesem Export."
fi

cat <<ENDE

Der Klon liegt unter $ZIEL und hat absichtlich kein Remote.

Veröffentlichen machst du selbst - bewusst nicht dieses Skript:

    cd $ZIEL
    git remote add origin git@github.com:<konto>/<repo>.git
    git push -u origin main

Vorher lohnt ein letzter eigener Blick:

    git log --oneline | head -20
    git log --all --pretty=format:%B | grep -i claude | head
ENDE
