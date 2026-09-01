# Workflow: vaccination → reminder

**Trigger** — a vaccination is recorded, manually or through a confirmed
document.

**Input** — pet, vaccine, date, optionally `next_due_date`, optionally a
document.

## Automatic steps

1. Store the vaccination with manufacturer, batch, diseases, vet and document.
2. **If and only if** `next_due_date` is present, create reminders at the
   user's configured lead times — default 30, 7 and 1 days before.
3. Write `vaccination.recorded`, and later `vaccination.due` when a lead time
   is reached.

## User review

Only where a document supplied the data: the next due date is a *proposal* and
has to be confirmed like any other extracted field.

## Output

One vaccination, its reminders, an entry in the pet timeline. The dashboard
shows it under "bald fällig" once the first lead time is reached.

## Failure cases

| Case | Behaviour |
| --- | --- |
| No next due date | **no reminder.** Not an error, not a gap to fill |
| Due date in the past | reminder fires immediately; overdue is a legitimate state |
| Vaccination corrected | reminders are recalculated, not duplicated |
| Pet deleted | reminders go with it |

## The line that must not be crossed

**HomeFlow never invents a vaccination interval.** Not from the vaccine name,
not from the species, not from what it saw last year. A next due date comes from
a document or from a person. Deriving one would be a medical recommendation, and
that is outside what this application is allowed to do.

## Idempotency

Unique constraint on (vaccination id, lead time). Re-processing produces no
second reminder. Changing the due date replaces the set rather than adding to it.
