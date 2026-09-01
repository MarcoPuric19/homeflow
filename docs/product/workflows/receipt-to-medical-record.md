# Workflow: receipt → medical record

The journey the product is judged on. Everything else is easier than this one.

**Trigger** — user confirms an extraction review for a document classified as a
vet receipt.

**Input** — the stored document, its OCR result, and the extraction candidates
the user has accepted or corrected.

## Automatic steps

1. Find or create the veterinarian. Match on practice name plus address; a
   near match is *proposed*, never applied silently.
2. Find an existing `medical_encounter` for this pet, this vet and this date. A
   receipt often arrives after the appointment already exists in the calendar.
3. If none exists, create one from the confirmed date, pet and practice.
4. Create `medical_records` for confirmed diagnoses, treatments and procedures,
   each linked to the encounter.
5. Link the document to the encounter through `document_links`.
6. Write `medical_encounter.created` to the outbox in the same transaction.

## User review

Everything medical passes through it. The screen shows field, value and
confidence, grouped so that the cheap checks come first:

    Tier      Nala                    ✓ 96 %
    Praxis    Tierarztpraxis XYZ         ✓ 98 %
    Datum     31.08.2026                 ✓ 99 %
    Diagnose  Gingivitis                 ? 72 %
    Behandlung Zahnuntersuchung          ? 87 %

Buttons: **Bestätigen · Korrigieren · Nicht übernehmen.**

Nothing medical is created before this. Confidence orders the list and marks
what to look at; it never decides.

## Output

One encounter, the confirmed medical records under it, the document linked, one
outbox event. The pet timeline shows a single entry, not eight.

## Failure cases

| Case | Behaviour |
| --- | --- |
| OCR finds nothing usable | `REVIEW_REQUIRED` with empty candidates; the review screen is a normal form |
| Receipt covers two animals | user assigns per position; two encounters, one document, linked to both |
| Practice unknown | proposal to create; free text remains allowed |
| Date unreadable | mandatory field in review, no default guess |
| User discards | document and OCR result stay, no domain data, status `CONFIRMED` with an empty result so it leaves the inbox |

## Idempotency

Unique constraint on (document id, target kind). A second confirmation of the
same document creates nothing new; it opens the existing encounter. The outbox
event carries the document id, so redelivery is a no-op.
