# Domain model

## Users

`User` has an immutable UUID, unique username, display name, Argon2id password
hash, role (`admin` or `member`), disabled flag and audit timestamps. There is
no public registration. Initial users are created by an explicit administrative
bootstrap process; later changes require an authenticated admin.

## Households

`Household` is the tenant boundary even though the local installation currently
contains exactly one household and two users. `HouseholdMember` binds a user to
that boundary. New domains must carry a mandatory `household_id`; repositories
derive it from the authenticated user's membership and never accept it from an
API request.

## Pets

`Pet` stores household-scoped master data: name, species, optional breed and
birth date with an explicit approximation flag, sex, optional microchip number,
color, insurance details, regular veterinarian, notes and an active flag. Archiving
is a reversible state transition; there is no destructive delete endpoint.
Pet lists use bounded keyset pagination. Medical records, vaccinations and
other pet subdomains will reference this aggregate in later increments.

`MedicalRecord` is one chronological timeline entry with a controlled type
(`diagnosis`, `treatment`, `procedure`, `examination`, `note`, `imaging` or
`report`), occurrence date, title, documented summary, optional veterinary
practice and bounded string details. It stores observations supplied by users;
HomeFlow neither infers diagnoses nor recommends treatment or dosage. A
composite foreign key guarantees that record and pet belong to the same
household. Timeline reads use descending keyset pagination.

`Vaccination` documents a vaccine name, administration date, optional
veterinary practice and batch number, notes and an optional manually supplied
next due date. HomeFlow never calculates an interval or decides whether a
vaccination is needed. History reads are household-scoped and keyset-paginated.

`Medication` contains household-scoped product master data. `PetMedication`
documents its assignment to one pet, including user-supplied dose and frequency
text, dates and an exact decimal stock quantity. No dose or consumption is
derived. Both foreign keys include `household_id`, and creation is atomic.

`Document` stores metadata for an untrusted medical JPEG, PNG or PDF and is
separate from the financial `Receipt` domain. It belongs to exactly one pet and
household, carries a controlled type, optional occurrence date, checksum and an
opaque server-generated storage key. The bytes share the existing hardened
local file adapter, but access is authorized through the pet relationship.
There is no inline preview, parser or OCR step.

`WeightEntry` and `LabResult` are append-only, household-scoped measurements.
Decimal values cross the API as strings and PostgreSQL stores them as exact
numeric values; HomeFlow does not classify results as normal or abnormal.
Their newest-first reads are capped at 100 rows.

Reminders are read models, not persisted jobs. A bounded query derives upcoming
manually entered vaccination due dates and planned pet calendar events for the
authenticated household. No scheduler, delivery channel or extra process is
involved.

`PetDashboard` is another read model. One household-scoped SQL statement returns
bounded counts, the newest weight, next manual vaccination due date, next
visible appointment and current-month financial totals. Detailed timelines are
still loaded only when opened; the dashboard does not duplicate domain state.
Financial month/year predicates use half-open date ranges so PostgreSQL can use
the household/date indexes; formatting a date inside a filter is avoided.

## Shopping

`ShoppingList` represents a shared or private list owned by one user. A
`ShoppingItem` contains name, quantity, unit, category, optional note, priority,
store, creator, completion state, buyer and timestamps. Completing an item is a
state transition, not deletion. Lists and items carry the household boundary.
`ShoppingItemPet` is a many-to-many relation so an item may target no pet, one
pet or several pets without duplicating the item.

## Calendar

`CalendarEvent` contains title, description, start/end instants, all-day flag,
category, owner, visibility, status, reminder and optional RFC 5545 RRULE.
Recurrence stores the rule rather than materializing an unbounded series.
Export will use iCalendar (`.ics`, RFC 5545).
An event may reference a pet in the same household and then carries a controlled
appointment type. Pet filtering is an additional predicate on the existing
bounded time-range query; no parallel appointment domain is introduced.

## Expenses

`Expense` contains description, merchant, planned and actual amounts in integer
minor currency units, ISO 4217 currency, due/paid dates, category, recurrence,
status, user, optional payment method and notes. Floating point is never used
for money. Every expense belongs to the caller's household and may reference a
pet from that same household plus a separate budget category. `PetBudget`
stores one integer-cent amount per household, pet, calendar month and category;
the composite unique key makes updates deterministic. Monthly and annual pet
summaries aggregate at most 100 categories and never scan an unbounded period.

## Receipts

`Receipt` belongs to an expense and records merchant, receipt date, total in
minor units, category, original filename, MIME type, SHA-256 checksum, upload
time, uploader and an opaque storage key. One expense may have many receipts.
Receipt access inherits the expense's household boundary before metadata or
bytes are returned.
OCR suggestions will be separate, untrusted records requiring confirmation.

## Domain events

Events use stable names such as `shopping.item.added`,
`shopping.item.completed`, `calendar.event.created`, `expense.created`,
`expense.due`, `expense.paid`, `receipt.uploaded` and `receipt.processed`.
Payloads are versioned and must not contain secrets.
