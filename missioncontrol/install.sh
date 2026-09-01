#!/usr/bin/env sh
# Installiert Mission Control als systemd-Benutzerdienst. Nichts hiervon läuft
# als root, und nichts wird ohne diesen ausdrücklichen Aufruf eingerichtet.
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
unit="$unit_dir/mission-control.service"

command -v systemctl >/dev/null 2>&1 || { echo "systemd wird benötigt (systemctl fehlt)." >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl wird benötigt." >&2; exit 1; }

echo "Baue Binary ..."
(cd "$repo/missioncontrol" && GOCACHE="$repo/.cache/go-build" GOMODCACHE="$repo/.cache/go-mod" \
  go build -trimpath -o bin/missioncontrol ./cmd/missioncontrol)

mkdir -p "$unit_dir"
sed "s|__REPO__|$repo|g" "$repo/missioncontrol/systemd/mission-control.service" > "$unit"
chmod 600 "$unit"

systemctl --user daemon-reload
systemctl --user enable --now mission-control.service

# Ohne linger startet ein Benutzerdienst erst bei der Anmeldung. Das ist für
# einen Laptop meist gewollt; wer ihn auch ohne Anmeldung will, schaltet linger
# selbst ein: loginctl enable-linger $USER
# Desktop-Eintrag: damit taucht Mission Control im Anwendungsmenü auf und
# startet in einem eigenen Fenster statt in einem Browser-Tab.
apps="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
icons="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/scalable/apps"
mkdir -p "$apps" "$icons"
sed "s|__REPO__|$repo|g" "$repo/missioncontrol/desktop/mission-control.desktop" > "$apps/mission-control.desktop"
chmod 644 "$apps/mission-control.desktop"
cp "$repo/missioncontrol/desktop/mission-control.svg" "$icons/mission-control.svg"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps" >/dev/null 2>&1 || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -f -t "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor" >/dev/null 2>&1 || true

echo
echo "Installiert: $unit"
echo "Menüeintrag: $apps/mission-control.desktop"
echo "Status:  systemctl --user status mission-control.service"
echo "Logs:    journalctl --user -u mission-control.service -f"
echo "Aus:     systemctl --user disable --now mission-control.service"
echo
echo "Cockpit: http://127.0.0.1:7777"
