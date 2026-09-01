# Pet domain v2: architecture

Architecture proposal for epic HF-0114. Read `../product/pet-platform-v2.md`
first for the why. **Nothing here is implemented.**

Existing constraints are inputs, not obstacles: modular monolith, PostgreSQL,
one laptop, 0 €, two users, `contracts.md` sections 1 and 2 unchanged unless
explicitly renegotiated.

## Components

    ┌─ homeflow-api (one binary, modular monolith) ──────────────┐
    │  httpapi        HTTP surface, authorisation, validation    │
    │  pets           pets, weight, vaccinations, medications    │
    │  encounters     NEW: MedicalEncounter, diagnosis, treatment│
    │  vets           NEW: veterinarian as an entity             │
    │  documents      files, links, types                        │
    │  extraction     NEW: candidates, confidence, review        │
    │  expenses       expenses, budgets — pet-bound              │
    │  insurance      NEW: policy, claim, reimbursement          │
    │  shopping       pet shopping list, inventory               │
    │  calendar       appointments, reminders                    │
    │  outbox         domain events (table exists, no writer)    │
    └────────────────────────────────────────────────────────────┘
                │ PostgreSQL                    │ shared file PVC
                ▼                               ▼
    ┌─ homeflow-worker (NEW, separate pod) ──────────────────────┐
    │  reads the job queue, runs OCR, writes candidates          │
    │  no database write access beyond its own job tables        │
    │  no external network, resource-limited, one job at a time  │
    └────────────────────────────────────────────────────────────┘

The worker is a separate process for one reason: it runs untrusted input
through a C library with an unpredictable memory profile, and that must not be
able to take the API down. It is not the start of a microservice architecture.

## Domain boundaries

| Module | Owns | May not |
| --- | --- | --- |
| `pets` | pet master data, weight, vaccination, medication | know about invoices or insurance |
| `encounters` | the visit and what happened medically | write expenses itself |
| `documents` | files, hashes, types, links | interpret content |
| `extraction` | OCR jobs, raw results, candidates, confidence | write domain data |
| `expenses` | money, budgets | know why the money was spent |
| `insurance` | policies, claims, reimbursements | change medical data |

The rule that keeps this honest: **`extraction` never writes domain data.** It
produces proposals. Only a confirmed review turns a proposal into a diagnosis,
an expense or a claim. That single boundary is what makes "no medical AI"
enforceable in code rather than in a policy document.

## MedicalEncounter is the missing grouping layer

Today a vet visit is up to eight unrelated rows. `medical_records` already
carries `record_type` (diagnosis, treatment, procedure, examination, …) and a
free-text `veterinarian`. What is missing is the bracket around them.

    MedicalEncounter
      pet, veterinarian, occurred_on, practice, summary, status
        ├── medical_records   (diagnosis, treatment, procedure)  ← exists today
        ├── vaccinations                                          ← exists today
        ├── medications / pet_medications                         ← exists today
        ├── lab_reports → lab_results                             ← results exist
        ├── weight_entries                                        ← exists today
        ├── documents (via document_links)                        ← partly exists
        ├── expenses                                              ← exists today
        └── insurance_claims                                      ← new

An encounter is **optional**. A weight entry on the sofa has no encounter; a vet
visit does. Making it mandatory would turn every quick action into a form, which
is the failure mode this whole redesign exists to avoid.

`veterinarian` becomes an entity with an optional free-text fallback, because
the practice on a receipt may be one HomeFlow has never seen. Extraction
proposes a match; the user confirms or creates.

## Document pipeline

    Upload ──► validate (size, MIME, magic bytes)
           ──► store under a server-generated key
           ──► SHA-256
           ──► duplicate check
           ──► create ocr_job
           ──► HTTP 202 { documentId, status: UPLOADED }

    Worker ──► claim job (SKIP LOCKED)
           ──► OCR engine ──► raw text + per-word confidence
           ──► parser ──► extraction candidates
           ──► status REVIEW_REQUIRED

    User   ──► review screen ──► confirm / correct / discard
           ──► domain data created inside one transaction

Status model: `UPLOADED → PROCESSING → REVIEW_REQUIRED → CONFIRMED`, plus
`FAILED` from any point. `FAILED` is not a dead end: the document exists, the
file is intact, and everything remains enterable by hand. **The application must
stay fully usable when OCR is wrong or absent** — that is a requirement, not a
fallback.

Raw OCR output and structured candidates are stored **separately**. The raw
result is evidence and stays; candidates are disposable and can be regenerated
by a better parser without touching the original.

## OCR architecture

    OcrEngine interface
        Extract(ctx, file) (OcrResult, error)

    OcrResult
        text, per-block confidence, engine name and version, duration

The interface already exists in `backend/internal/ocr` from the F4 work
(provider abstraction with four outcomes) and is the right shape. What is
missing is an implementation with a real engine behind it.

Engine candidates were assessed in HF-0074; **Tesseract 5 (Apache-2.0)** with an
offline `deu.traineddata` is first choice: mature on printed receipts, CPU only,
smallest image, no runtime model download. Preprocessing (deskew, contrast,
downscale) matters more than engine choice for phone photos and comes first.

The engine stays behind its own gate. See "Sequencing" below.

## Worker architecture

Queue in PostgreSQL, not a broker. `SELECT … FOR UPDATE SKIP LOCKED` with a
visibility timeout, an attempt counter and a dead-letter state.

- **One job at a time.** Reliability over throughput on this hardware.
- **Timeout per job**, enforced by the worker and by the container limit.
- **Idempotent by construction**: a job carries the document id; a completed job
  is never reprocessed into a second set of candidates.
- The worker has **no external network** and no write access to domain tables.

## Event flows

`event_outbox` exists and has no writer (a known trap in AGENTS.md). This is
what fills it:

    document.uploaded          ocr.completed / ocr.failed
    document.confirmed         medical_encounter.created
    vaccination.recorded       vaccination.due
    medication.stock.low       appointment.upcoming
    expense.created            budget.threshold.reached
    insurance_claim.created    insurance_claim.reimbursed
    shopping_item.completed

Events are written **in the same transaction** as the state change that caused
them. A dispatcher inside the API reads the outbox and reacts. No Kafka, no
broker, no second infrastructure component.

Idempotency is a schema property, not a convention: unique constraints on
(source kind, source id, target kind) so that one confirmed receipt cannot
produce two expenses, two encounters or two claims — no matter how often the
event is delivered.

## Data model

New tables, on top of the existing 26:

    veterinarians              practice, vet, address, contact
    medical_encounters         pet, vet, date, practice, summary, status
    encounter_items            polymorphic link: encounter ↔ record/vaccination/
                               medication/lab/weight/expense/claim
    lab_reports                report above the existing lab_results
    document_links             document ↔ any domain object, replaces the two
                               special-case tables
    ocr_jobs                   status, attempts, timing, engine
    ocr_results                raw text and confidence, one per job
    extraction_candidates      field, value, confidence, source region, state
    pet_insurance_policies     provider, number, coverage, deductible
    insurance_claims           encounter, amounts, status
    insurance_reimbursements   payment against a claim
    inventory_items            stock independent of medication
    notifications              in-app, generic

Changes to existing tables: `expenses.pet_id` and `calendar_events.pet_id`
become NOT NULL after migration; `receipts` gains a pet reference;
`medical_records` gains an optional `encounter_id`; `document_appointments` and
`document_medical_records` are replaced by `document_links`.

Deliberately **not** done: no JSON blob for core data (`medical_records.details`
already exists and stays for genuinely open attributes only), no separate
diagnosis and treatment tables — `medical_records.record_type` already
distinguishes them and splitting would be normalisation without benefit.

## Security boundaries

Everything in `../security/threat-model.md` applies unchanged. New surface:

- **Uploads are hostile.** Size limit, MIME check, magic bytes,
  server-generated storage keys, SHA-256, no execution, no user-controlled
  paths, no inline rendering. Already implemented for receipts; extended to all
  documents.
- **The OCR worker is the blast radius.** It parses attacker-controlled files
  in a C library. Non-root, read-only root filesystem, all capabilities
  dropped, seccomp RuntimeDefault, temporary workspace as `emptyDir`, memory
  and CPU limits as a control rather than a hint, egress DNS only, no database
  access beyond its job tables.
- **Extracted text is untrusted input**, exactly like the file it came from. It
  is escaped on display and validated before it reaches any domain field.
- **Audit trail** on medical records, medications, vaccinations, receipts and
  insurance claims: who, when, what. Not every UI action. No secrets, no
  document content, ever.
- The two-user limit and `HOMEFLOW_ENV` restrictions stay untouched.

## Sequencing, and the honest constraint

Everything above except the engine works without OCR and is useful on its own.
The engine is gated on the benchmark from HF-0074, because HF-0084 deferred it
on measured grounds: 0 % text layer on real receipts, and no memory to spare.

The gate is a number, not an opinion: **Tesseract with preprocessing has to stay
within roughly 256 MiB and a few seconds per page**, and it has to beat typing
three fields by hand — fifteen seconds — including correction. If it does not,
the receipt review is built anyway and fed manually, and the engine waits for
hardware.
