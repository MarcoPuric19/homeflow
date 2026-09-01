#!/usr/bin/env sh
# Startet Mission Control als eigenständiges Fenster.
#
# Das ist bewusst kein natives GTK-Fenster: dafür bräuchte es cgo und
# webkit2gtk, also genau die Abhängigkeiten, die dieses Werkzeug nicht haben
# soll. Stattdessen wird ein vorhandener Browser im App-Modus benutzt - eigenes
# Fenster, eigenes Icon in der Leiste, keine Adresszeile, keine Tabs.
set -eu

url="http://127.0.0.1:7777"
repo=$(cd "$(dirname "$0")/../.." && pwd)

# 1. Dienst sicherstellen. Läuft er als Benutzerdienst, wird er gestartet;
#    sonst wird ein Vordergrundprozess angeworfen, der mit dem Fenster endet.
started_here=""
if ! curl -sf -o /dev/null --max-time 2 "$url/api/health" 2>/dev/null; then
  if command -v systemctl >/dev/null 2>&1 && systemctl --user list-unit-files mission-control.service >/dev/null 2>&1; then
    systemctl --user start mission-control.service || true
  else
    "$repo/missioncontrol/bin/missioncontrol" >/dev/null 2>&1 &
    started_here=$!
  fi
  # Auf Bereitschaft warten, statt in ein leeres Fenster zu starten.
  i=0
  while [ "$i" -lt 30 ]; do
    curl -sf -o /dev/null --max-time 1 "$url/api/health" 2>/dev/null && break
    i=$((i + 1))
    sleep 0.3
  done
fi

# 2. Fenster öffnen. Der App-Modus ist die einzige Zutat, die aus einer Webseite
#    ein Fenster ohne Browser-Beiwerk macht.
for browser in chromium chromium-browser google-chrome google-chrome-stable brave-browser microsoft-edge; do
  if command -v "$browser" >/dev/null 2>&1; then
    exec "$browser" --app="$url" --class=mission-control --window-size=1400,900
  fi
done

# Firefox kennt keinen App-Modus (SSB wurde entfernt). Ein eigenes Fenster geht
# trotzdem; wer die Browserleisten loswerden will, setzt MC_KIOSK=1 und bekommt
# ein randloses Vollbild - dann beendet F11 bzw. Strg+W das Fenster.
if command -v firefox >/dev/null 2>&1; then
  if [ "${MC_KIOSK:-0}" = "1" ]; then
    exec firefox --kiosk "$url"
  fi
  exec firefox --new-window "$url"
fi

# Letzter Ausweg: irgendetwas, das URLs öffnen kann.
if command -v xdg-open >/dev/null 2>&1; then
  exec xdg-open "$url"
fi

echo "Kein Browser gefunden. Mission Control läuft unter $url" >&2
[ -n "$started_here" ] && wait "$started_here"
exit 1
