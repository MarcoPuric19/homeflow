# Workflow: appointment

**Trigger** — an appointment is created manually, from a reminder, or as a
follow-up proposed by a confirmed document.

**Input** — pet, date and time, type, veterinarian, notes.

## Automatic steps

1. Create the calendar event with a **mandatory** pet reference — after the v2
   migration there are no calendar entries without a pet.
2. Create reminders at the configured lead times.
3. Emit `appointment.upcoming` when a lead time is reached.
4. After the appointment, offer to create a `medical_encounter` — or link the
   existing one when a receipt has already produced it.

## User review

Creation is a normal form, kept as short as possible: pet, date, type. Vet and
notes are prefilled from the pet's treating vet and are optional.

## Output

Calendar event, reminders, and after the visit a link to the encounter — so
that appointment, receipt and medical record are one thing in the timeline
rather than three.

## Failure cases

| Case | Behaviour |
| --- | --- |
| Appointment moved | reminders recalculated, not duplicated |
| Appointment cancelled | reminders removed; an already created encounter is **not** deleted |
| Receipt arrives before the appointment is marked done | the receipt finds the appointment by pet, vet and date and links to it |
| Two appointments on the same day at the same practice | the receipt cannot decide; the user picks |

## Idempotency

The receipt workflow searches for an existing encounter by (pet, vet, date)
before creating one. Unique constraint on (appointment id, lead time) for the
reminders.
