#!/usr/bin/env sh
# Misst, was eine lokale OCR-Engine auf DIESER Maschine an echten Belegen
# kostet - das Tor aus HF-0084: rund 256 MiB und wenige Sekunden je Seite.
#
# HF-0074 verlangt gemessenen statt übernommenen Bedarf. Deshalb läuft alles im
# selben Image, das später als Pod ausgerollt würde, mit hartem Speicherlimit
# und ohne Netz - was hier gemessen wird, gilt auch im Betrieb.
#
# Zwei Betriebsarten je PDF, weil Vorverarbeitung bei Scans mehr bringt als die
# Enginewahl: 'roh' rendert mit 300 dpi in Farbe, 'sparsam' mit 300 dpi in
# Graustufen. Fotos aus dem Messenger liegen bereits bei rund 2 MP und werden
# unverändert gelesen.
#
# Liest ausschließlich. Das Belegverzeichnis wird nur eingehängt, nur lesend.
set -eu

: "${IMAGE:=homeflow/ocr-bench:1}"
: "${MEMORY:=512m}"
: "${DPI:=300}"

[ $# -eq 1 ] || { echo "Aufruf: $0 <verzeichnis-mit-belegen>" >&2; exit 64; }
QUELLE=$(cd "$1" && pwd) || exit 66
command -v docker >/dev/null 2>&1 || { echo "docker fehlt" >&2; exit 69; }
docker image inspect "$IMAGE" >/dev/null 2>&1 || {
  echo "Image $IMAGE fehlt. Bauen mit:" >&2
  echo "  docker build -t $IMAGE -f docker/ocr/Dockerfile docker/ocr" >&2
  exit 69; }

AUS=$(mktemp -d)
# Der Container läuft als unprivilegierter Benutzer (uid 65532) und muss in das
# Ausgabeverzeichnis schreiben können. Ohne das scheitert jeder Lauf lautlos -
# genau so ist der erste Durchgang mit lauter Nullen durchgelaufen.
chmod 0777 "$AUS"
trap 'rm -rf "$AUS"' EXIT INT TERM

# Ein Lauf: Datei rendern und erkennen, Zeit und Speicherspitze mitschreiben.
# --network none ist keine Vorsichtsmaßnahme fürs Messen, sondern die Bedingung
# aus HF-0083: das Modell liegt im Image, sonst wird die Engine nicht benutzt.
lauf() {
  datei=$1; modus=$2
  docker run --rm --network none --memory="$MEMORY" \
    -v "$QUELLE":/in:ro -v "$AUS":/out \
    --entrypoint sh "$IMAGE" -c '
      set -e
      datei="/in/$1"; modus="$2"; dpi="$3"; kennung="$4"
      cd /tmp
      case "$datei" in
        *.pdf|*.PDF)
          if [ "$modus" = sparsam ]; then
            pdftoppm -gray -r "$dpi" "$datei" seite
          else
            pdftoppm -r "$dpi" "$datei" seite
          fi ;;
        *) cp "$datei" seite-1.jpg ;;
      esac
      : > /out/"$kennung".txt
      spitze=0; sekunden=0; seiten=0
      for bild in seite-*; do
        seiten=$((seiten+1))
        # --psm 4 (eine Spalte variabler Textgrößen), nicht 6 (ein Block).
        # Belege sind mehrspaltig - Kopf, Positionstabelle, Summenblock. Mit
        # psm 6 verschränkt Tesseract die Spalten und der Praxisname zerfällt;
        # mit psm 4 kommt er sauber heraus. Gleicher Speicher, gleiche Zeit,
        # deutlich besserer Text (HF-0074).
        /usr/bin/time -f "%M %e" -o /tmp/mess \
          tesseract "$bild" /tmp/out -l deu --psm 4 >/dev/null 2>&1 || true
        cat /tmp/out.txt >> /out/"$kennung".txt 2>/dev/null || true
        kb=$(cut -d" " -f1 /tmp/mess); s=$(cut -d" " -f2 /tmp/mess)
        [ "$kb" -gt "$spitze" ] && spitze=$kb
        sekunden=$(awk -v a="$sekunden" -v b="$s" "BEGIN{printf \"%.1f\", a+b}")
      done
      echo "$seiten $spitze $sekunden" > /out/"$kennung".mess
    ' sh "$datei" "$modus" "$DPI" "$3" || {
      # Fehler nicht verschlucken: eine Null in der Tabelle sieht aus wie ein
      # Messwert und ist keiner.
      echo "FEHLGESCHLAGEN: $datei ($modus)" >&2
      echo "0 0 0" > "$AUS/$3.mess"
    }
}

printf '%-44s %-8s %5s %9s %8s %8s\n' Datei Modus Seiten "RAM-Spitze" Sekunden Zeichen
printf '%s\n' "--------------------------------------------------------------------------------------"

i=0
for pfad in "$QUELLE"/*; do
  [ -f "$pfad" ] || continue
  name=$(basename "$pfad")
  case "$name" in
    *.pdf|*.PDF) modi="roh sparsam" ;;
    *.jpg|*.jpeg|*.JPG|*.JPEG|*.png|*.PNG) modi="roh" ;;
    *) continue ;;
  esac
  for modus in $modi; do
    i=$((i+1)); kennung="lauf$i"
    lauf "$name" "$modus" "$kennung"
    read -r seiten spitze sekunden < "$AUS/$kennung.mess"
    zeichen=$(wc -c < "$AUS/$kennung.txt" 2>/dev/null || echo 0)
    printf '%-44.44s %-8s %5s %6s MiB %8s %8s\n' \
      "$name" "$modus" "$seiten" "$((spitze/1024))" "$sekunden" "$zeichen"
    cp "$AUS/$kennung.txt" "$AUS/$(echo "$name" | tr ' /' '__')-$modus.txt" 2>/dev/null || true
  done
done

ZIEL=${ERGEBNIS:-/tmp/ocr-benchmark}
mkdir -p "$ZIEL"
cp "$AUS"/*.txt "$ZIEL"/ 2>/dev/null || true
echo
echo "Erkannter Text je Datei: $ZIEL"
echo "Tor aus HF-0084: rund 256 MiB und wenige Sekunden je Seite."
