# ADR 0008: CI/CD stays local, and the repository gets an offline mirror

- Status: Accepted
- Date: 2026-09-01
- Decision owner: Marco
- Ticket: HF-0096 (epic HF-0095)

## Context

The working title for epic F5 was "GitHub CI/CD". Three written rules stand
against it: AGENTS.md rule 2 forbids cloud services, `docs/security/threat-model.md`
forbids remote access, and this repository deliberately has no remote — the
history exists exactly once, on one laptop.

Cost is not the obstacle. A private GitHub repository is free, needs no payment
details, and its Actions quota would never be reached by this project. The
obstacle is what would leave the machine: not the household's data — `.env`,
backups and secrets are and stay git-ignored — but the domain model, the
architecture, and a threat model that documents which security gates are *not*
met.

Two questions are easy to conflate and were separated before deciding:

1. **Where do the checks run?** Today on this laptop, through `make ci`, an
   optional pre-push hook, and Mission Control's deploy pipelines.
2. **Where does a second copy of the history live?** Today: nowhere. Two commits,
   one machine — a machine that OOM-killed six containers under memory pressure
   on the day this was written.

Only the second question describes an actual present risk. The first describes a
convenience the project does not currently need: there is one human author, and
review happens through Mission Control tickets with path claims rather than
through pull requests.

Four options were weighed (HF-0096): GitHub with hosted runners; GitHub as a
remote only; a local bare mirror with checks staying local; and a self-hosted
Forgejo. Forgejo satisfies every rule and fails on memory — roughly 250 to
400 MiB permanently, on a machine whose budget is documented as exhausted in
`docs/operations/resource-budget.md`.

## Decision

**Checks stay local. The repository gets one or more offline mirrors, and
mirroring is automated rather than remembered.**

- Every check is a Makefile target or a script under `scripts/`, runnable by
  hand on this laptop. Any future CI surface calls those targets and contains no
  logic of its own. This keeps the location replaceable and makes every failure
  reproducible locally.
- Mirrors are ordinary git remotes whose name begins with `spiegel`.
  `scripts/mirror-repo.sh` pushes to all of them and reports honestly which ones
  were reachable. No mirror configured is a loud failure, not a silent success.
- Nothing is published anywhere.

The decisive argument is asymmetric reversibility. Going from local to GitHub
later is one `git remote add` and one push. Going the other way is not possible:
code that has been on GitHub survives in forks, caches and indexes. Between two
otherwise close options, the reversible one goes first.

## Triggers for revisiting

This decision is scoped to the beta. It is to be reconsidered — as a new ADR,
not as a quiet drift — when any of these becomes true:

1. **A second person writes code.** Ticket claims work between two assistants
   coordinated by one human. They do not scale to reviewers who are not in the
   room.
2. **The system leaves the laptop.** Remote access has its own gates in
   `docs/security/threat-model.md`; if those are ever met, the reasoning here
   changes with them.
3. **F2 (HF-0002) starts.** Ten thousand users is not a laptop, and at that
   point hosted CI and a real registry stop being ceremony.
4. **Off-site redundancy becomes a requirement.** See the consequence below.

## Consequences

There is no pull-request review, no second pair of eyes outside this machine,
and no execution of the checks on foreign hardware — so "works here" and "works
on a clean machine" are not distinguished. That gap is accepted for the beta and
is one of the reasons the checks are being made reproducible and hermetic
(HF-0097 through HF-0103).

**The mirror protects against disk failure and against a destroyed working tree.
It does not protect against fire, theft or flood** if it lives in the same flat.
That is a real limit of this decision, not an oversight. If off-site redundancy
is wanted, the next step is option B — GitHub as an encrypted remote with
`git-crypt` or `age`, checks still local — and not option A.

Mirroring only preserves what has been committed. At the time of this decision
the working tree held over a hundred changed files against two commits, which is
a larger exposure than the missing remote. Commit frequency is the cheapest
improvement available and is a habit, not a tool.

The 0-euro rule is untouched: a bare repository costs nothing, needs no account,
no trial and no permanently running service.
