# ADR 0007: Pet budget warnings use fixed minor currency units

- Status: Accepted
- Date: 2026-09-01
- Decision owner: Marco
- Ticket: HF-0069

## Context

Pet budgets need an optional local warning without external delivery. The
threshold could be expressed as either a percentage or a money amount. All
existing HomeFlow finance values use integer minor currency units and avoid
floating point arithmetic.

## Decision

Each monthly pet budget may store one non-negative warning threshold in the
same integer minor currency unit as the budget. The warning is reached when
`budget amount - actually paid expenses <= warning threshold`. Planned and open
expenses remain visible but do not trigger the warning. Annual summaries sum
monthly budget amounts and configured thresholds per category before applying
the same comparison. A missing threshold never triggers a warning.

## Consequences

The result is exact, explainable and consistent with the existing money model.
Users choose the amount separately for every category and month. Thresholds
are not directly comparable as percentages, and changing the budget does not
automatically scale the threshold. The warning remains local UI state; it does
not add a scheduler, notification service or external integration.
