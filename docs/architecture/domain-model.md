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

The combined pet timeline is a read-only projection across medical records,
vaccinations, medication assignments, weights, lab results, documents and pet
appointments. It is fetched with one bounded `UNION ALL` query and paginated
newest-first by date, source type and source id. Its title is source data, not
an automated medical interpretation or summary.

Each household stores one to ten unique reminder lead times. The default set is
30, 7 and 1 days. Reminder reads emit a stable item for the exact configured
stage reached by a vaccination due date or pet appointment; there is no
scheduler, delivery state or external notification component.

A vaccination reminder may be converted into a calendar appointment only by
an explicit user request that supplies title, time and visibility. The event is
owned by the caller, references the same pet and has appointment type
`vaccination`. `reminder_appointments` provides a durable one-to-one idempotency
key for the source vaccination; foreign-household sources are indistinguishable
from missing ones.

Medication inventory may additionally store an exact positive package size, an
exact non-negative minimum stock and manual purchase/expiry dates. Quantities
share the documented stock unit. These fields never trigger dose or consumption
calculations.

When documented medication stock is below its manual minimum, users may first
request a read-only restock preview and then explicitly create a pet-linked
shopping item. A partial unique index permits only one open item per medication
assignment under races; completing it permits a later item. Stock itself is
updated through a separate exact, manual replacement operation.
Financial month/year predicates use half-open date ranges so PostgreSQL can use
the household/date indexes; formatting a date inside a filter is avoided.

## Shopping

`ShoppingList` represents a shared or private list owned by one user. A
`ShoppingItem` contains name, quantity, unit, category, optional note, priority,
store, creator, completion state, buyer and timestamps. Completing an item is a
state transition, not deletion. Lists and items carry the household boundary.
`ShoppingItemPet` is a many-to-many relation so an item may target no pet, one
pet or several pets without duplicating the item.
Items may additionally document a preferred brand, an estimated integer-minor-
unit price with currency and whether the purchase is recurring. Recurrence is
descriptive only and never materialises another item automatically.

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
the composite unique key makes updates deterministic. It may additionally hold
a fixed warning threshold in integer cents and private notes. The remaining
budget is the budget amount minus actually paid expenses; a warning is reached
when that value is less than or equal to the threshold. Monthly and annual pet
summaries aggregate budgets and thresholds into at most 100 categories and
never scan an unbounded period.

## Receipts

`Receipt` belongs to an expense and records merchant, receipt date, total in
minor units, category, original filename, MIME type, SHA-256 checksum, upload
time, uploader and an opaque storage key. One expense may have many receipts.
Receipt access inherits the expense's household boundary before metadata or
bytes are returned.
OCR suggestions will be separate, untrusted records requiring confirmation.

## Domain events

Pet workflows transactionally append version-1 events to `event_outbox` using
the stable names `pet.created`, `pet.updated`, `pet.weight.recorded`,
`medical_record.created`, `vaccination.recorded`, `appointment.created`,
`appointment.completed`, `medication.started`, `medication.stopped`,
`medication.stock.low`, `pet_shopping_item.created`, `pet_expense.created`,
`pet_budget.threshold.reached` and `receipt.attached`. Every payload contains
only stable household/pet correlation plus identifiers needed for routing; it
must not contain names, notes, medical values, filenames, document bytes or
secrets. Event creation and the corresponding domain write share one database
transaction. No publisher is currently attached to the outbox.
