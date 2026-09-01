# HomeFlow v2: what happens to the existing code

Step 5 to 7 of epic HF-0114: the existing code judged against the new goal, a
migration plan without a big bang, and a release order.

**Nothing here is implemented.** Awaiting approval.

## Classification

| Module | Lines | Verdict | Why |
| --- | --- | --- | --- |
| `auth` | 650 | **KEEP** | Two-user limit, Argon2, sessions. Untouched by the product change. |
| `pets` | 1977 | **KEEP + ADD** | The core, already correct. Gains encounter references. |
| `reminders` | 311 | **KEEP** | Derived reminders and rules, already pet-bound. |
| `observability` | 128 | **KEEP** | Metric names are a contract (`contracts.md` 1). |
| `postgres` | 104 | **KEEP** | Pool of 5, migrations under advisory lock. |
| `integration` | 733 | **KEEP** | Adapters stay optional. The pet domain must never depend on them. |
| `ocr` | 654 | **KEEP + ADD** | Provider interface with four outcomes is the right shape. Missing: a real engine. |
| `httpapi` | 2658 | **REFACTOR** | 55 routes become the v2 surface; `/pets/{id}/timeline`, `/documents`, `/inbox` are new. |
| `calendar` | 730 | **REFACTOR** | `pet_id` becomes mandatory. Everything else stays. |
| `expenses` | 559 | **REFACTOR** | `pet_id` becomes mandatory; budgets stay derived from expenses. |
| `shopping` | 521 | **REFACTOR** | The generic "Gemeinsame Einkaufsliste" goes; items become pet-bound with categories. |
| `documents` | 251 | **REFACTOR** | Gains types, `document_links` and the OCR job reference. |
| `outbox` | 56 | **REFACTOR** | The table exists and has no writer. It gets one. |
| `receipts` | 517 | **MIGRATE** | Merges into `documents`. Upload validation, magic bytes and storage keys are carried over unchanged — they are the security-relevant part. |
| `links` | 281 | **MIGRATE** | Two special-case link tables become one general `document_links` plus `encounter_items`. |
| `domain`, `platform` | 0 | **REMOVE** | Empty directory skeletons that were never filled. |
| — | — | **ADD** | `encounters`, `vets`, `extraction`, `insurance`, the worker binary |

Nothing is thrown away. The two MIGRATE entries move code, they do not rewrite
it.

## What "remove the household stuff" concretely means

Four cuts, each one migration:

1. **Expenses without a pet.** `expenses.pet_id` becomes NOT NULL. Existing
   rows without one are shown to the user for assignment before the constraint
   is applied.
2. **The generic shared shopping list.** `SharedListID` and the "shared"
   fallback disappear; items belong to one, several or all pets.
3. **Calendar entries without a pet.** `calendar_events.pet_id` becomes NOT
   NULL, same assignment step.
4. **Receipts without a pet.** They merge into `documents`, which is pet-bound.

`households` and `household_members` stay. They are the tenancy boundary that
every authorisation query and the two-user limit rest on. That is a schema
concern, not a product one.

## Migration principles

- **No big bang.** Each release is deployable on its own and leaves the
  application usable.
- **Every migration is reversible.** `.down.sql` for each, checked by
  `make check-migrations`.
- **Data assignment before constraint.** Where a column becomes NOT NULL, the
  user assigns the orphans first. No silent default, no invented pet.
- **Numbers reserved first.** Migration numbers go into `contracts.md` 1.3
  before the file is written — the collision is silent otherwise, and now the
  check enforces it.
- **Interfaces and quantities in the same pass**, per the golden rule.

## Release order

This deviates from the order in the brief in two places, both deliberate.

| Release | Content |
| --- | --- |
| **V2.0** | Trim: pet-bound expenses, calendar and shopping; six-entry navigation; dashboard skeleton |
| **V2.1** | `Veterinarian` entity, `MedicalEncounter`, pet timeline as a read model over both |
| **V2.2** | Document central: `receipts` merged in, `document_links`, document types, inbox |
| **V2.3** | Outbox writer, domain events, idempotency constraints |
| **V2.4** | Expenses and budgets against encounters; manual receipt → expense |
| **V2.5** | OCR — **the gate is open**, see below: worker, job queue, extraction, review |
| **V2.6** | Insurance: policy, claim, reimbursement |
| **V2.7** | Medication, inventory, pet shopping list, restock suggestions |
| **V2.8** | Vaccinations and reminder automation |
| **V2.9** | UX, performance, security hardening; audit trail completion |

### The OCR gate opened on 2026-09-01

ADR 0009 measured it on the real documents: 71 MiB peak against a 256 MiB
budget, 2.4 to 4.8 seconds per scan. HF-0084's trigger is met, so OCR moved from
V2.8 to V2.5 — directly behind expenses, which is what it writes into. The
paragraph below explains why it was placed last while the number was unknown; it
is kept because the reasoning still applies to the next gated decision.

### Why timeline and encounters are one release, not two

The brief has V2.0 profiles and timeline, V2.1 encounters. A timeline built
before encounters exist groups nothing and gets regrouped in the next release.
That is churn on the most visible screen in the product. HF-0064 already built a
bounded timeline; V2.1 extends it rather than replacing it twice.

### Why events move from last to the middle

Every workflow in `workflows/` depends on outbox events and idempotency
constraints. Building expenses, insurance and medication first and retrofitting
events afterwards means writing the coupling twice, and the second time under
the pressure of existing data. V2.3 is the cheapest point: after documents exist
(so there is something to react to), before the three consumers are written.

### Why OCR moves from V2.2 to V2.8

This is the deviation that needs the clearest justification, because OCR is the
soul of the product.

It is **gated, not deprioritised.** HF-0084 deferred the engine on measured
grounds: 0 % of the real receipts have a text layer (HF-0073), so the cheap path
does not apply, and the memory is not there
(`../operations/resource-budget.md`). The gate is the benchmark from HF-0074 —
Tesseract with preprocessing inside roughly 256 MiB and a few seconds per page,
beating fifteen seconds of typing including correction.

**The benchmark can run at any time and should run early.** If it passes, OCR
moves forward in this list — the review screen from V2.8 is the only part that
depends on the engine, and everything it writes into already exists by V2.5.
If it fails, the review screen is still built and fed by hand, and the engine
waits for hardware rather than for a decision.

Building it in V2.2 as the brief suggests would mean either shipping an engine
that does not fit, or blocking releases V2.3 through V2.7 behind a benchmark
none of them need.

## What each release owes the next one

    V2.0 ──► clean pet-bound data          every later release assumes it
    V2.1 ──► encounter as grouping         timeline, receipts, insurance need it
    V2.2 ──► documents and links           OCR, insurance, medical records need it
    V2.3 ──► events and idempotency        every workflow needs it
    V2.4 ──► expenses on encounters        insurance and budgets need it
    V2.5 ──► insurance                     nothing depends on it
    V2.6 ──► medication and shopping       nothing depends on it
    V2.7 ──► vaccination reminders         nothing depends on it
    V2.8 ──► OCR                           improves V2.2 and V2.4, required by none

The last four are independently orderable. If the OCR benchmark passes early,
V2.8 can move directly behind V2.4.
