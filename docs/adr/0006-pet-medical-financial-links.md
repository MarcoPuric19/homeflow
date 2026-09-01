# ADR 0006: Pet medical and financial relationships use explicit link tables

- Status: Accepted
- Date: 2026-09-01
- Ticket: HF-0063

## Context

HomeFlow must relate pet documents, calendar appointments, medical records,
expenses and receipts while keeping medical data independent from financial
data. Every relation must be tenant-safe and, where applicable, refer to the
same pet. A receipt already belongs to exactly one expense; a medical document
belongs to exactly one pet.

Adding `expense_id` to medical records would make the medical domain depend on
finance. Adding several nullable foreign keys to every entity would also limit
each relation to one target and make same-household/same-pet constraints easy
to bypass in application code.

## Decision

Use explicit relationship tables:

- `document_medical_records`
- `document_appointments`
- `expense_medical_records`
- `expense_appointments`

Each row carries `household_id` and `pet_id` in addition to both related IDs.
Composite foreign keys enforce that both sides belong to that household and
pet. Expense links require the expense to have a pet; general household
expenses cannot be attached to a pet medical record accidentally.

Receipts remain related only to their expense. Their appointment and medical
record relationships are obtained through that expense, so receipt storage and
authorization do not gain a second access path. Documents remain independent
of expenses and may be linked to medical records or appointments without any
financial object.

The API exposes relationships as bounded ID collections on the document and
expense resources. Linking is an explicit authenticated user action. It does
not create medical records, expenses or appointments implicitly.

## Consequences

- Medical history remains usable without any expense.
- Multiple documents and expenses can relate to the same visit or record.
- Tenant and pet isolation is enforced by PostgreSQL, not only handlers.
- Four small link tables and composite unique constraints are required.
- Deletes remove relationship rows, never the related domain object.
- Timeline and workflow features can consume these links later without schema
  changes.

## Rejected alternatives

- Medical records depending on expenses: violates the domain boundary.
- Nullable foreign keys on all domain tables: one-to-one limitation and a
  growing, asymmetric schema.
- Generic polymorphic relation table: PostgreSQL cannot enforce target foreign
  keys, weakening security for private documents.
- Event-only relationships: the current outbox has no writer and events are not
  an authoritative query model.

## Cost and operations

The design uses the existing local PostgreSQL instance and API process. It adds
no service, account, network access or paid dependency. Runtime RAM is
unchanged; storage overhead is limited to UUID relationship rows and indexes.
