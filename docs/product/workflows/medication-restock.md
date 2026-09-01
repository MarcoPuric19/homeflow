# Workflow: medication running low → shopping item

**Trigger** — the stock of a `pet_medication` falls below its minimum, whether
through a scheduled dose being logged or through a manual correction.

**Input** — pet medication with stock, minimum stock, unit and package size.

## Automatic steps

1. On every stock change, compare against the minimum.
2. Below it, emit `medication.stock.low` and create a **suggestion** — not a
   shopping item.
3. The suggestion appears in the inbox and on the dashboard under Einkauf.

## User review

One tap: *"Medikament nachkaufen?"* → the item lands on the pet shopping list,
prefilled with name, quantity by package size, unit, pet and the medication it
came from.

Deliberately not automatic. A shopping list that fills itself is a shopping list
people stop trusting, and the whole product depends on people trusting what it
proposes.

## Output

One shopping item, linked to pet and source medication. After purchase,
optionally an expense and a stock increase.

## Failure cases

| Case | Behaviour |
| --- | --- |
| Stock corrected back above minimum | open suggestion disappears; an accepted item stays, because the person may still want it |
| Medication discontinued | suggestion is withdrawn, the shopping item is not — it is the user's list |
| No minimum stock set | no suggestion, ever. Silence is correct here |
| Package size unknown | quantity 1, unit from the medication |

**No automatic ordering.** There is no purchasing integration and none is
planned.

## Idempotency

At most one open suggestion per pet medication, enforced by a partial unique
index. Repeated dips below the minimum do not stack.
