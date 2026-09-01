#!/usr/bin/env sh
# Beantwortet die erste Frage aus HF-0073: wie viele unserer PDFs haben schon
# eine Textschicht?
#
# Wo eine vorhanden ist, ist OCR überflüssig - direkte Extraktion ist schneller,
# exakter und braucht weder Modell noch Pod. Fällt der Anteil hoch aus, ändert
# das den Zuschnitt des ganzen OCR-Epics (HF-0072).
#
# Liest ausschließlich. Verändert keine Datei und schreibt nichts in das
# untersuchte Verzeichnis.
set -eu

# Ab wie vielen Zeichen je Seite gilt eine Textschicht als brauchbar. Ein
# gescanntes Blatt liefert meist 0 bis wenige Zeichen (Ränder, Stempel), ein
# echter Beleg mehrere hundert.
: "${MIN_CHARS_PER_PAGE:=100}"

usage() {
  echo "Aufruf: $0 <verzeichnis-mit-pdfs>" >&2
  echo "  MIN_CHARS_PER_PAGE=$MIN_CHARS_PER_PAGE (Schwelle je Seite)" >&2
  exit 2
}

[ $# -eq 1 ] || usage
directory=$1
[ -d "$directory" ] || { echo "Kein Verzeichnis: $directory" >&2; exit 2; }

for tool in pdfinfo pdftotext pdfimages file; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Fehlt: $tool (poppler-utils bzw. file)" >&2; exit 2; }
done

# Für eine Seite ohne Text ist die nächste Frage, WAS dort liegt. Ein einzelnes
# großes Bild je Seite ist ein Foto oder Scan - dann bestimmt seine Auflösung,
# wie viel Speicher eine OCR-Engine je Seite braucht und wie gut sie liest.
# Diese Zahl entscheidet in HF-0074 mehr als der Name der Engine.
# Maße eines gewöhnlichen Bildes. `file` liest nur den Kopf, lädt also nicht
# das ganze Bild - das ist genau die Prüfung, die auch vor dem Dekodieren
# stattfinden muss (HF-0083).
image_dimensions() {
  # `file` nennt bei JPEG mehrere Zahlenpaare: erst die Auflösung in dpi
  # ("density 200x200"), dann die eigentlichen Maße ("1653x2339"). Das erste
  # Paar zu nehmen war falsch - ein 3,9-MP-Foto erschien als 0,04 MP.
  # Genommen wird deshalb das Paar mit dem größten Produkt; die dpi-Angabe ist
  # in jedem für OCR sinnvollen Bild kleiner als die Maße.
  file -b "$1" 2>/dev/null | grep -oE '[0-9]+ ?x ?[0-9]+' | tr -d ' ' |
    awk -Fx '{if ($1*$2 > best) {best=$1*$2; win=$0}} END {if (win != "") print win}'
}

# Megapixel der größten Seite - eine Zahl je Datei, für die Auswertung unten.
page_megapixel() {
  pdfimages -list "$1" 2>/dev/null | tail -n +3 |
    awk -v pages="$2" '{s+=($4+0)*($5+0)} END {if (pages>0 && s>0) printf "%.2f\n", s/pages/1000000}'
}

image_note() {
  file=$1
  pages=$2
  images=$(pdfimages -list "$file" 2>/dev/null | tail -n +3)
  [ -n "$images" ] || { echo "kein Bild"; return; }

  count=$(echo "$images" | grep -c .)
  # Spalten von pdfimages -list: page num type width height color comp bpc enc ... x-ppi y-ppi
  widest=$(echo "$images" | awk '{if ($4+0 > max) {max=$4+0; h=$5; ppi=$13}} END {printf "%dx%s @%sdpi", max, h, ppi}')
  megapixel=$(echo "$images" | awk '{s+=($4+0)*($5+0)} END {printf "%.1f", s/1000000}')
  printf '%s Bild(er), größtes %s, gesamt %s MP' "$count" "$widest" "$megapixel"
}

total=0
with_text=0
partial=0
without_text=0
unreadable=0

printf '%-40s %5s %8s %6s  %-16s %s\n' "Datei" "Seit." "Zeichen" "je S." "Einstufung" "Bildseiten"
printf '%s\n' "------------------------------------------------------------------------------------------------"

# find statt Glob: Unterverzeichnisse und Dateinamen mit Leerzeichen. Bilder
# gehören dazu - ein abfotografierter Beleg ist derselbe Fall wie ein Scan im
# PDF, nur ohne Umschlag.
find "$directory" -type f \
  \( -iname '*.pdf' -o -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.png' \
     -o -iname '*.tif' -o -iname '*.tiff' -o -iname '*.webp' -o -iname '*.heic' \) \
  -print | sort | while IFS= read -r file; do
  total=$((total + 1))
  name=$(basename "$file")

  case "$file" in
    *.pdf|*.PDF|*.Pdf) ;;
    *)
      # Ein Bild hat per Definition keine Textschicht. Die interessante Zahl
      # ist seine Größe.
      dimensions=$(image_dimensions "$file")
      if [ -z "$dimensions" ]; then
        printf '%-40s %5s %8s %6s  %-16s %s\n' "$(echo "$name" | cut -c1-40)" "?" "?" "?" "unlesbar" "-"
        unreadable=$((unreadable + 1))
      else
        megapixel=$(echo "$dimensions" | awk -Fx '{printf "%.2f", ($1*$2)/1000000}')
        printf '%-40s %5s %8s %6s  %-16s %s\n' \
          "$(echo "$name" | cut -c1-40)" "1" "0" "0" "Bild -> OCR" "$dimensions, $megapixel MP"
        without_text=$((without_text + 1))
        echo "$megapixel" >> /tmp/ocr-mp.$$
      fi
      echo "$total $with_text $partial $without_text $unreadable" > /tmp/ocr-count.$$
      continue
      ;;
  esac

  pages=$(pdfinfo "$file" 2>/dev/null | awk '/^Pages:/ {print $2}')
  if [ -z "${pages:-}" ] || [ "$pages" -lt 1 ] 2>/dev/null; then
    # Kaputt, verschlüsselt oder kein PDF - zählt als eigene Kategorie, nicht
    # als "braucht OCR": das wäre eine falsche Aussage über die Datei.
    printf '%-40s %5s %8s %6s  %-16s %s\n' "$(echo "$name" | cut -c1-40)" "?" "?" "?" "unlesbar" "-"
    unreadable=$((unreadable + 1))
    echo "$total $with_text $partial $without_text $unreadable" > /tmp/ocr-count.$$
    continue
  fi

  chars=$(pdftotext -q "$file" - 2>/dev/null | tr -d '[:space:]' | wc -c | tr -d ' ')
  per_page=$((chars / pages))

  if [ "$per_page" -ge "$MIN_CHARS_PER_PAGE" ]; then
    verdict="Textschicht"
    with_text=$((with_text + 1))
  elif [ "$chars" -gt 0 ]; then
    # Etwas Text, aber zu wenig: typisch für Scans mit Kopfzeile oder für
    # gemischte Dokumente. Die brauchen OCR, aber nicht für alles.
    verdict="teilweise"
    partial=$((partial + 1))
  else
    verdict="kein Text -> OCR"
    without_text=$((without_text + 1))
  fi

  printf '%-40s %5s %8s %6s  %-16s %s\n' \
    "$(echo "$name" | cut -c1-40)" "$pages" "$chars" "$per_page" "$verdict" "$(image_note "$file" "$pages")"
  page_megapixel "$file" "$pages" >> /tmp/ocr-mp.$$
  echo "$total $with_text $partial $without_text $unreadable" > /tmp/ocr-count.$$
done

# Die Schleife läuft in einer Subshell (Pipe), deshalb die Zwischendatei.
if [ -f /tmp/ocr-count.$$ ]; then
  read -r total with_text partial without_text unreadable < /tmp/ocr-count.$$
  rm -f /tmp/ocr-count.$$
fi

echo
if [ "${total:-0}" -eq 0 ]; then
  echo "Keine PDF-Datei in $directory gefunden."
  exit 0
fi

printf 'Untersucht: %s Dateien (Schwelle %s Zeichen je Seite)\n' "$total" "$MIN_CHARS_PER_PAGE"
printf '  Textschicht brauchbar : %s (%s%%)\n' "$with_text" "$((with_text * 100 / total))"
printf '  teilweise Text        : %s (%s%%)\n' "$partial" "$((partial * 100 / total))"
printf '  kein Text, OCR nötig  : %s (%s%%)\n' "$without_text" "$((without_text * 100 / total))"
printf '  unlesbar              : %s\n' "$unreadable"
echo
skipped=$(find "$directory" -type f \
  ! -iname '*.pdf' ! -iname '*.jpg' ! -iname '*.jpeg' ! -iname '*.png' \
  ! -iname '*.tif' ! -iname '*.tiff' ! -iname '*.webp' ! -iname '*.heic' | wc -l | tr -d ' ')
if [ "$skipped" -gt 0 ]; then
  printf '  übergangen           : %s Dateien mit anderer Endung\n' "$skipped"
  find "$directory" -type f \
    ! -iname '*.pdf' ! -iname '*.jpg' ! -iname '*.jpeg' ! -iname '*.png' \
    ! -iname '*.tif' ! -iname '*.tiff' ! -iname '*.webp' ! -iname '*.heic' \
    -printf '      %f\n' 2>/dev/null | head -5
fi
echo
echo "Lesart: 'Textschicht brauchbar' braucht keine OCR-Engine, sondern nur"
echo "pdftotext. Ist dieser Anteil hoch, deckt der billige Pfad den Großteil ab"
echo "und die Engine wird nur noch für Fotos und echte Scans gebraucht."

if [ -s /tmp/ocr-mp.$$ ]; then
  echo
  echo "Bildseiten - die Grundlage für Engine-Wahl und Pod-Grenzen (HF-0074, HF-0076):"
  sort -g /tmp/ocr-mp.$$ | awk '
    {v[NR]=$1; sum+=$1}
    END {
      if (NR==0) exit
      printf "  Megapixel je Seite: kleinste %.1f, Median %.1f, größte %.1f\n", v[1], v[int((NR+1)/2)], v[NR]
      # Roher Speicher der dekodierten Seite, reine Rechnung: Breite mal Höhe
      # mal Bytes je Pixel. Was eine Engine obendrauf braucht, ist zu messen.
      printf "  Dekodiert je größter Seite: %.0f MB grau (1 B/px), %.0f MB RGB (3 B/px)\n", v[NR], v[NR]*3
      if (v[NR] > 8)
        printf "  ACHTUNG: über 8 MP je Seite. Herunterrechnen auf rund 300 dpi und Graustufen\n         vor der Erkennung spart ein Vielfaches, ohne dass gedruckte Belege\n         schlechter gelesen werden.\n"
    }'
  rm -f /tmp/ocr-mp.$$
fi
