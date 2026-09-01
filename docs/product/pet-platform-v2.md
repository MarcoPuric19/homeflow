# HomeFlow v2: a private pet record

Product refinement for epic HF-0114. This document describes what the product
becomes and why; `../architecture/pet-domain-v2.md` describes how, and
`workflows/` describes each flow in detail.

**Nothing here is implemented yet.** It is a proposal awaiting approval.

## What it is

A private pet record for two people, self-hosted, on one laptop. Not a SaaS
product, no public registration, no third party holding the data. The existing
rules stand unchanged: 0 €, local storage, no paid APIs, no cloud OCR, exactly
two administratively managed users.

## The one principle everything else follows from

**Capture information once.**

A vet visit produces a receipt, a diagnosis, a treatment, possibly medication,
possibly lab work, a cost, a budget entry, possibly an insurance claim and
possibly a follow-up appointment. Today that is up to eight separate manual
entries. It should be one:

    CAPTURE  →  EXTRACT  →  REVIEW  →  CONFIRM  →  AUTOMATE

Photograph the receipt. HomeFlow proposes what it read. The user checks it for
twenty seconds. Confirm. Done.

Every screen is measured against one question: *what does HomeFlow already
know?* Known, prefill it. Reliably derivable, propose it. Uncertain, ask.
Redundant, delete the field.

## Where the product stands today

The analysis is less dramatic than the brief suggests. HomeFlow is already
mostly a pet application:

| | |
| --- | --- |
| API routes under `/pets` | 27 of 55 |
| Tables carrying a `pet_id` | 13 of 26 |
| Backend modules that are pet-only | pets, reminders, links |
| Backend modules that are pet-*capable* | calendar, expenses, shopping, documents |

Removing "the household stuff" therefore means trimming at a few specific
points, not rebuilding.

### households stays — as a boundary, not as a subject

Every table hangs off `household_id`. The two-user limit is enforced in the
database (migration `000002`), and every authorisation query goes through
`household_members`. That is the tenancy anchor, and removing it would be a
migration across the entire schema for no product gain.

What disappears is the household as *subject matter*: expenses with no pet, the
generic shared shopping list, calendar entries with no pet, receipts not tied to
a pet. The word stays in the schema; it leaves the interface.

### What is genuinely missing

`Veterinarian` as an entity (today it is free text on `medical_records`),
`MedicalEncounter` as the grouping layer, the whole insurance domain,
`LabReport` above the existing `lab_results`, the OCR job and extraction
candidate tables, a general document link table, notifications, and inventory
separate from medication stock.

## Navigation

Six entries. Not one per table.

    Dashboard   Tiere   Kalender   Einkauf   Finanzen   Dokumente

Vaccinations, diagnoses, medications and lab values are **not** navigation
entries. They live inside the pet they belong to. A person does not think "let
me open the vaccination list"; they think "what does Nala still need".

The Inbox is not a seventh entry — it is the top of the dashboard, because
"what needs my attention" and "what is next" are the same question.

## Dashboard

Answers one question: **what do we have to do next for our animals?**

| Block | Content |
| --- | --- |
| Inbox | receipt to review, document unassigned, insurance claim open |
| Heute | medication due, appointments, tasks due today |
| Bald fällig | vaccinations, check-ups, medication running out, recurring exams |
| Einkauf | open pet items, low stock |
| Finanzen | current pet costs, open invoices, expected reimbursements, budget warnings |

Quick actions, with receipt scanning first and largest:

    + Rechnung scannen   + Termin   + Gewicht   + Medikament gegeben   + Einkauf

No technical information on this page. No sync status, no job counters, no
version numbers.

## Pet profile

Header: photo, name, species, breed, sex, age, date of birth, chip number,
current weight, insurance, treating vet.

Directly below, before any tab: **active conditions, allergies, active
medication, special notes.** These are the things someone needs at a vet's
counter with a phone in one hand and a cat box in the other.

Tabs: Übersicht · Patientenakte · Medikamente · Impfungen · Termine · Labor ·
Dokumente · Kosten · Versicherung.

## The medical record is a timeline

Chronological, mixing event types, with the structured data intact underneath:

    01.09.2026  Tierarztbesuch — Praxis XYZ
                Diagnose · Behandlung · Medikamente
                Dokumente: Rechnung.pdf, Labor.pdf
                184,30 €  ·  Versicherung: eingereicht

    25.08.2026  Gewicht 5,4 kg
    14.08.2026  Impfung Purevax RCP

The timeline is a **read model**, not a table. It is assembled from the
underlying entities so that the presentation can change without a migration.
HF-0064 already built a bounded version of exactly this; it is the foundation,
not a competitor.

## User journeys, in order of how often they happen

1. **Vet receipt** — photograph, review, confirm. Everything else follows.
   This is the journey the product is judged on.
2. **Medication given** — one tap from the dashboard, stock decreases, low
   stock proposes a shopping item.
3. **Weight** — one number, one tap.
4. **Vaccination due** — reminder, plan appointment, later the vaccination
   record with its document.
5. **Shopping** — suggestion accepted with one tap, checked off while shopping,
   optionally becomes an expense.
6. **Insurance** — after a confirmed receipt: create claim? Everything is
   already linked; the user adds nothing.

## Priorities

**Now, without OCR** — the pet record works fully manually and is useful on its
own: pet profile, timeline, MedicalEncounter, veterinarian entity, document
central with links, expenses and budgets tied to pets, insurance domain,
medication with inventory, vaccinations with reminders, pet shopping list.

**Gated on a benchmark** — the OCR engine, receipt review, extraction
candidates, confidence display. See the honest constraint below.

**Deliberately later** — smart home adapters, mobile app, notifications beyond
in-app.

## The constraint that shapes the whole plan

The central feature depends on OCR. OCR is deferred in HF-0084 for a measured
reason: **0 % of the real receipts have a text layer** (HF-0073), so the cheap
extraction path does not apply to actual documents, and a real engine costs
memory this machine does not have spare
(`../operations/resource-budget.md`).

This is not a reason to abandon the vision. It is a reason to sequence it so
that everything except the engine is built first — each piece useful on its own
— and the engine passes through its own gate: the benchmark from HF-0074, which
HF-0084 itself names as the cheaper of the two triggers.

The honest counter-question from HF-0074 stands and belongs in the benchmark's
acceptance criteria: three fields typed by hand take fifteen seconds. OCR has to
beat that *including* correction, or it is ceremony.

## What is deliberately not built

- **No medical intelligence.** The system organises and extracts. It does not
  diagnose, does not recommend therapy, does not calculate dosages, does not
  interpret lab values and does not invent vaccination intervals. "Amoxicillin
  was recognised on the receipt" is allowed. "The animal should get
  Amoxicillin" is not.
- **No automatic adoption of medical data.** Every extracted medical fact
  passes through human confirmation. Confidence is a display aid, never an
  authority.
- **No automatic ordering**, no purchase integrations.
- **No insurance provider API or scraping** without explicit consent. The
  domain is provider-neutral; adapters come later or never.
- **No microservices.** The OCR worker is a second process because it is
  resource-isolated, not because the architecture wants distribution.
- **No paid dependency, no free tier requiring a card, no telemetry.**
