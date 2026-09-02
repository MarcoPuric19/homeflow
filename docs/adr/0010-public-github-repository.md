# ADR 0010: The repository becomes public on GitHub; operations stay local

- Status: accepted
- Date: 2026-09-02
- Decision owner: Marco
- Supersedes: parts of [ADR 0008](0008-ci-cd-location.md)

## Context

ADR 0008 decided, four weeks into the beta, that CI/CD would stay local and the
repository would get an offline mirror rather than a forge account. The reasons
were cost control, no third-party accounts, and the fact that nothing about the
delivery path actually needed a remote: the cluster runs on the same laptop the
code is written on.

That reasoning still holds for *operations*. It never covered *visibility*.

Marco is looking for work as an SRE. Employers increasingly read repositories
instead of CVs, and a project that exists only on one laptop cannot be read.
The value of this repository as evidence — measured resource budgets, alert
rules tested for effect, a deploy that verifies what it shipped — is lost if
nobody can open it.

## Decision

The repository becomes **publicly readable on GitHub**. Operations do not move.

Concretely:

- **GitHub gets a derived clone, not the original.** The local repository stays
  the source of truth, keeps its full history, and keeps the offline mirror
  from ADR 0008. `scripts/public-export.sh` produces the public clone by
  rewriting history; it verifies the result and deliberately does not push.
- **CI runs on GitHub Actions and executes `make ci`** — the same target used
  on the laptop, not a reimplementation. A remote check that verifies less than
  the local one is worse than none.
- **No deployment runs from GitHub.** `make deploy` continues to target the
  local kind cluster. No cluster credentials, no kubeconfig, and no runtime
  secrets exist in the repository or in Actions. If GitHub is unavailable,
  development and operation are unaffected.
- **The public artifact is code, not data.** No credentials, no receipts, no
  household or pet data. `docs/security/threat-model.md` is unchanged: nothing
  becomes reachable beyond loopback.

## Why this stays within the free-of-charge rule

GitHub Actions, CodeQL and Dependabot are free without limit **for public
repositories**. No card, no trial, no additional account beyond the one Marco
already has. This is load-bearing: for a *private* repository, Actions minutes
are metered, and the same design would violate rule 2. Publishing is what makes
the CI free, not a side effect of it.

## Consequences

- History becomes rewritable-once and then permanent. Anything pushed is
  forked, cached and indexed; deletion afterwards is not a remedy. The export
  script therefore fails closed: any excluded path or trailer still present in
  the rewritten history aborts the run.
- The public and local histories diverge by construction. The local repository
  keeps the assistant instructions; the public one does not.
- Commit subjects continue to reference ticket ids that are not resolvable from
  the public repository alone. This is accepted — it reads like a reference to
  an internal tracker, which is what it is.
- A published mistake cannot be recalled. Every change to the exclusion list is
  a decision for Marco, not for an assistant.

## Triggers for revisiting

- If the repository ever needs to be private again, the free-CI premise falls
  away and the workflows need to be re-costed against Actions minutes.
- If a deployment ever should run from CI, this ADR does not authorise it; the
  gates in `docs/security/threat-model.md` apply.
