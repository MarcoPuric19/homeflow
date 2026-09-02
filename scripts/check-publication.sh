#!/usr/bin/env sh
# Vorabpruefung: was ginge nach draussen, wenn jetzt veroeffentlicht wuerde?
#
# Anders als scripts/public-export.sh baut das hier nichts und aendert nichts.
# Es liest die vorhandene Historie und sagt, was darin steht. Damit laesst sich
# der Stand jederzeit pruefen, ohne git-filter-repo zu installieren.
#
# Gepruefte Fragen, in der Reihenfolge ihrer Schwere:
#   1. Geheimnisse und Zahlungsdaten - duerfen nie drin sein
#   2. Werte aus redactions.local.txt - stehen noch in alten Commits
#   3. Auszuschliessende Pfade      - werden erst vom Export entfernt
#   4. Was danach lesbar bleibt     - zur Kenntnis, keine Wertung

set -eu

wurzel="$(cd "$(dirname "$0")/.." && pwd)"
cd "$wurzel"
schwaerzungen="$wurzel/redactions.local.txt"

rot()  { printf '\033[31m%s\033[0m\n' "$*" >&2; }
info() { printf '  %s\n' "$*"; }

fehler=0
verlauf="$(git log --all -p 2>/dev/null || true)"

if [ -z "$verlauf" ]; then
  rot "check-publication: keine Historie gefunden"
  exit 1
fi

echo "check-publication: Historie mit $(git rev-list --all --count) Commits"
echo

# --- 1. Geheimnisse -------------------------------------------------------
echo "Geheimnisse und Zahlungsdaten:"
for muster in 'BEGIN [A-Z ]*PRIVATE KEY' '\bDE[0-9]{20}\b' '\b[0-9]{16}\b'; do
  n="$(printf '%s' "$verlauf" | grep -cE "$muster" || true)"
  if [ "$n" -gt 0 ]; then
    rot "  SPERRE: $n Treffer fuer /$muster/"
    fehler=1
  fi
done
[ "$fehler" -eq 0 ] && info "keine Schluessel, IBANs oder Kartennummern"

# --- 2. Schwaerzungen -----------------------------------------------------
echo
echo "Zu schwaerzende Werte:"
if [ -f "$schwaerzungen" ]; then
  offen=0
  while IFS= read -r zeile; do
    case "$zeile" in ''|\#*) continue ;; esac
    original="${zeile%%==>*}"
    [ -z "$original" ] && continue
    n="$(printf '%s' "$verlauf" | grep -cF -- "$original" || true)"
    if [ "$n" -gt 0 ]; then
      info "noch in $n Zeile(n) der Historie: $original"
      offen=$((offen + 1))
    fi
  done < "$schwaerzungen"
  if [ "$offen" -gt 0 ]; then
    info ""
    info "$offen Wert(e) stehen in alten Commits. Das ist erwartbar - der"
    info "Arbeitsbaum ist bereits bereinigt. public-export.sh ersetzt sie beim"
    info "Rewrite und prueft danach nach. OHNE diesen Export gingen sie mit raus."
  else
    info "keiner der Werte steht noch in der Historie"
  fi
else
  info "keine redactions.local.txt - es ist kein Textersatz konfiguriert"
fi

# --- 3. Auszuschliessende Pfade -------------------------------------------
echo
echo "Pfade, die der Export entfernt:"
alle="$(git log --all --pretty=format: --name-only | sort -u | sed '/^$/d')"
for p in AGENTS.md CLAUDE.md Austausch.md; do
  n="$(printf '%s\n' "$alle" | grep -cx "$p" || true)"
  if [ "$n" -gt 0 ]; then
    info "in der Historie vorhanden (wird vom Export entfernt): $p"
  fi
done
n="$(git log --all --pretty=format:%B | grep -ciE '^[[:space:]]*Co-Authored-By:' || true)"
[ "$n" -gt 0 ] && info "$n Co-Authored-By-Trailer (werden vom Export entfernt)"

# --- 4. Was sichtbar bleibt ----------------------------------------------
echo
echo "Bleibt nach dem Export oeffentlich lesbar:"
zaehle() { git grep -I -c "$1" -- . 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'; }
info "$(zaehle 'Claude Code') Fundstellen 'Claude Code' im Arbeitsbaum"
info "$(zaehle 'Codex') Fundstellen 'Codex'"
info "$(git ls-files missioncontrol/tickets | wc -l | tr -d ' ') Ticketdateien"
adressen="$(printf '%s' "$verlauf" \
  | grep -ohE '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-z]{2,}' \
  | grep -viE 'example|noreply|localhost|\.local|@AGENTS|@CLAUDE|git@github' \
  | sort -u | tr '\n' ' ' || true)"
info "E-Mail-Adressen: ${adressen:-keine}"

echo
if [ "$fehler" -ne 0 ]; then
  rot "check-publication: NICHT veroeffentlichungsfaehig - siehe Sperren oben."
  exit 2
fi
echo "check-publication: keine Sperre. Der Rest ist eine Entscheidung, keine Pruefung."
