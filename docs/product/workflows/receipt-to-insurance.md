# Workflow: receipt → insurance claim

**Trigger** — after a confirmed receipt, when the pet has an active policy.
The user is asked once; they are never asked to re-enter anything.

**Input** — encounter, document, expense, policy.

## Automatic steps

1. Check for an active `pet_insurance_policy` for this pet on the service date.
2. If there is one, offer: *"Versicherungsfall erstellen?"*
3. On yes, create an `insurance_claim` in status `NOT_SUBMITTED`, linked to
   encounter, receipt, expense and documents, with the invoice amount prefilled.
4. Write `insurance_claim.created` to the outbox.

## User review

One question, one tap. The claim opens prefilled; the claimed amount defaults
to the invoice amount and is editable. Submission itself is manual — status
moves to `SUBMITTED` when the user says so, because HomeFlow does not talk to
any insurer.

Status model:

    NOT_SUBMITTED → READY_TO_SUBMIT → SUBMITTED → IN_REVIEW
                 → PARTIALLY_REIMBURSED | REIMBURSED | REJECTED

Stored per claim: invoice amount, claimed amount, reimbursed amount, deductible.
A reimbursement is its own row, so partial payments are a normal case and not an
overwrite.

## Output

One claim, everything already linked. The dashboard shows open claims and
expected reimbursements.

## Failure cases

| Case | Behaviour |
| --- | --- |
| No policy | no question, no claim; nothing to explain to the user |
| Policy expired on the service date | no automatic claim; a hint that a policy exists but did not cover that date |
| Rejected | status `REJECTED` with a reason field; the expense stays untouched |
| Partially reimbursed | reimbursement row, claim status derived from the sum, never edited by hand |

## Idempotency

Unique constraint on (encounter id, policy id). One visit under one policy is
one claim. Answering the question twice opens the existing claim.

**Explicitly not built:** no insurer API, no scraping, no upload automation.
Adapters under `integrations/insurance/` remain an interface with no
implementation until Marco says otherwise.
