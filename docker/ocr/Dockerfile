# syntax=docker/dockerfile:1
# OCR-Umgebung für den Benchmark aus HF-0074 und später für den Pod aus HF-0076.
#
# Bewusst selbst gebaut statt ein fremdes Image von Docker Hub zu ziehen: die
# Herkunft ist damit nachvollziehbar (Debian-Paketquellen), und es ist genau das
# Artefakt, das später ausgerollt würde - der Benchmark misst dann nicht etwas
# anderes als der Betrieb.
#
# Das Sprachmodell liegt IM Image. Eine Engine, die Modelle zur Laufzeit
# nachlädt, scheidet aus: der Pod bekommt kein externes Netz (HF-0083).
FROM debian:trixie-slim

# tesseract-ocr-deu ist das deutsche Modell, poppler-utils liefert pdftoppm für
# die Seitenumwandlung, time misst die Speicherspitze je Lauf (HF-0074 verlangt
# gemessenen statt geschätzten Bedarf). Kein Python, kein PyTorch, keine GPU - alles davon wäre
# auf 7 GiB Laptop-RAM der falsche Weg.
RUN apt-get update \
 && apt-get install --no-install-recommends -y \
      tesseract-ocr tesseract-ocr-deu poppler-utils time \
 && rm -rf /var/lib/apt/lists/*

# Für den Betrieb später: kein root, kein Schreibrecht außerhalb des
# Arbeitsbereichs. Für den Benchmark ändert es nichts, aber es soll nicht erst
# nachträglich dazukommen.
RUN useradd --system --uid 65532 --no-create-home ocr
USER 65532
WORKDIR /work
ENTRYPOINT ["tesseract"]
