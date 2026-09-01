# ADR 0009: Tesseract 5, grayscale at 300 dpi, page segmentation mode 4

- Status: Accepted
- Date: 2026-09-01
- Decision owner: Marco
- Ticket: HF-0074 (epic HF-0072)

## Context

Epic F4 needs local OCR: no cloud service, no paid API, no account, no model
downloaded at runtime. HF-0084 deferred the engine on measured grounds — 0 % of
the real receipts carry a text layer (HF-0073), so the cheap extraction path
does not apply, and the memory budget is exhausted
(`../operations/resource-budget.md`).

HF-0084 set the gate explicitly: **roughly 256 MiB and a few seconds per page**,
and it has to beat typing three fields by hand — about fifteen seconds —
including correction.

Measured on the actual laptop against Marco's ten real documents: four scanned
PDFs (one and two pages, up to 10.7 MP per page) and six phone photos at about
2 MP. Reproducible with `scripts/ocr-benchmark.sh`.

## Decision

**Tesseract 5.5.0 (Apache-2.0) with the offline German model, pages rendered at
300 dpi in grayscale, page segmentation mode 4.**

The image is built from Debian package sources
(`docker/ocr/Dockerfile`) rather than pulled from a third party: the provenance
is traceable, and it is the same artefact that would later run as the pod, so
the benchmark measures what production would do. 77 MB, model included, no
network at runtime.

## Measurements

| Class | Memory peak | Time |
| --- | --- | --- |
| Scan, 2 pages, grayscale | 67–71 MiB | 3.5–4.8 s |
| Scan, 1 page, grayscale | 67–68 MiB | 2.4–2.6 s |
| Scan, colour (for comparison) | 125–132 MiB | 2.9–6.1 s |
| Phone photo, ~2 MP | 51–61 MiB | 0.2–1.4 s |

**The gate is passed with roughly a factor of three to spare.** The worst case
measured is 71 MiB against 256 MiB.

Two findings that cost nothing and matter more than the engine choice:

**Grayscale halves the memory and changes nothing else.** 125 MiB colour versus
67 MiB grayscale, with a character-for-character identical result on every
scanned page. Rendering receipts in colour buys nothing.

**Page segmentation mode 4 beats mode 6 on receipts.** Receipts are
multi-column — header, item table, totals block. `--psm 6` treats the page as
one uniform block and interleaves the columns, which shreds the practice name.
`--psm 4` reads a single column of varying sizes and returns it cleanly. Same
memory, same time. The first benchmark run used mode 6 and made Tesseract look
worse than it is; that was a parameter mistake, not an engine property.

## Quality

Eight of ten documents yield practice, date and total; several also yield the
patient line with name, species, breed, date of birth and chip number:

    Rechnungsdatum : 12.08.2026
    12.08.2026 Patient: Katze (Mischling) Mila (Geb:03.04.2021) (Chip: 2760…)
    Summe Brutto | 112,40 €

Two documents fail. One is a phone **screenshot** rather than a photo of a
receipt — the recognised text is the status bar and the navigation bar, so there
was never receipt content to read. The other yields a practice name but no date.

**This is a presence check, not an accuracy measurement.** The benchmark shows
that the fields are in the text; a crude regular expression pulled two totals
out visibly wrong. Real accuracy needs ground truth from Marco, per document.
That measurement is still owed and must not be replaced by these numbers.

Errors already visible in the recognised text, and they are the interesting
part: a year read as 2028 instead of 2026, another as 2025, "Nala" as
"Naia", and decimal separators inconsistently as comma or full stop. These
are exactly the mistakes that look plausible on screen. They are the reason the
architecture puts a human confirmation between extraction and domain data
(`pet-domain-v2.md`), and the reason the review screen shows a formatted amount
rather than raw digits.

## Consequences

The engine is affordable. HF-0084's trigger is met, so OCR can move forward in
the v2 release order from V2.8 to directly behind V2.4 — the decision is
Marco's, the measurement no longer blocks it.

The worker pod gets a **memory limit of 256 MiB** with a request near the
measured peak, one job at a time, no external network, no database access, model
in the image. That is a quantity in the sense of `contracts.md` section 2 and is
recorded there when the pod is built.

Rejected alternatives: PaddleOCR, docTR and EasyOCR — all free, all Apache-2.0,
all bringing model sizes and memory needs that this measurement makes
unnecessary. There is no reason to spend more when 71 MiB is enough. Cloud OCR
was never a candidate.

What this ADR does **not** decide: whether OCR is worth it in daily use. The
counter-question from HF-0074 stands — three fields by hand are fifteen seconds,
and the review still costs time. That is answered by using it, not by measuring
it.
