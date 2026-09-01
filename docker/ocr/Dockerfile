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
FROM debian:trixie-slim AS betrieb

# tesseract-ocr-deu ist das deutsche Modell (Apache-2.0, tessdata_fast),
# poppler-utils liefert pdftoppm für die Seitenumwandlung. Kein Python, kein
# PyTorch, keine GPU - alles davon wäre auf 7 GiB Laptop-RAM der falsche Weg.
#
# poppler ist das einzige Copyleft-Stück (GPL-2/GPL-3). Es wird bewusst als
# eigenes Programm aufgerufen und nicht in ein Binary gelinkt.
RUN apt-get update \
 && apt-get install --no-install-recommends -y \
      tesseract-ocr tesseract-ocr-deu poppler-utils \
 && rm -rf /var/lib/apt/lists/*

# Kein root, kein Schreibrecht außerhalb des Arbeitsbereichs - dieselbe Härtung
# wie beim Rest des Clusters, von Anfang an statt nachträglich.
RUN useradd --system --uid 65532 --no-create-home ocr
USER 65532
WORKDIR /work
ENTRYPOINT ["tesseract"]

# Zweite Stufe nur für den Benchmark aus HF-0074. GNU time misst die
# Speicherspitze, ist GPL und wird im Betrieb nicht gebraucht - was nicht
# gebraucht wird, gehört nicht ins Image.
#
#   Betrieb:   docker build --target betrieb   -t homeflow/ocr:1 ...
#   Benchmark: docker build --target benchmark -t homeflow/ocr-bench:1 ...
#
# BEIDE Aufrufe brauchen --target. Ohne baut Docker die LETZTE Stufe, und das
# ist hier der Benchmark - die Zielangabe wegzulassen liefert also still das
# falsche Image.
#
# Vorher stand hier eine ARG-Bedingung im apt-Aufruf. Sie hat nicht gewirkt, das
# Paket landete in beiden Images, und man sah es dem Dockerfile nicht an. Zwei
# Stufen sind länger und dafür überprüfbar - `test -x /usr/bin/time` beantwortet
# die Frage in einer Zeile.
FROM betrieb AS benchmark
USER root
RUN apt-get update \
 && apt-get install --no-install-recommends -y time \
 && rm -rf /var/lib/apt/lists/*
USER 65532
