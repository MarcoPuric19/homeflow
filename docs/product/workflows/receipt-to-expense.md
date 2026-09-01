# Workflow: receipt → expense

**Trigger** — same confirmation as `receipt-to-medical-record.md`. One user
action produces both; there is never a second finance form.

**Input** — confirmed total, date, pet, veterinarian, document.

## Automatic steps

1. Create an expense: pet, category `veterinary`, amount in integer minor
   units, date, document reference, encounter reference.
2. Link expense and encounter, so the timeline can show the cost inline.
3. Write `expense.created` to the outbox in the same transaction.

The amount stays exactly as confirmed. No rounding, no currency conversion, no
derived net or gross figures — `contracts.md` fixes integer minor units for all
money and that does not change here.

## User review

None of its own. The amount was confirmed in the receipt review, and asking
twice is precisely the pattern this redesign removes. The expense is editable
afterwards like any other.

## Output

One expense, linked to pet, encounter and document. Visible under the pet's
costs and in the finance view.

## Failure cases

| Case | Behaviour |
| --- | --- |
| Total unreadable | mandatory field in review; without a confirmed amount no expense is created and the encounter is created without one |
| Decimal separator misread (184.30 vs 18430) | the review shows the formatted amount, not raw digits, so the error is visible rather than plausible |
| Split receipt across two pets | one expense per pet with its own share; the user assigns, the system does not divide by itself |
| Amount corrected later | normal edit; the budget derives from expenses and follows automatically |

## Idempotency

Unique constraint on (document id, kind `expense`). One document, one expense.
Re-running the event finds the existing row.
