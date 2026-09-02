# Third-party components and licenses

HomeFlow is bound by a project rule that predates this document: **everything
must be free of charge, permanently, without an account, a trial, or a card**
(`AGENTS.md`, rule 2). That rule already excluded Alexa and Ring
([ADR 0005](../adr/0005-integrations.md)).

This file is the evidence for that claim, and it is deliberately narrow: only
direct dependencies and deployed runtime components. It is checked
mechanically — see [Keeping this honest](#keeping-this-honest).

Every license below was read from the artifact itself (module cache,
`node_modules`, upstream repository), not from memory or a package index
summary.

## Build and runtime dependencies

### Go — `backend/`

| Module | Version | License |
| --- | --- | --- |
| `github.com/jackc/pgx/v5` | v5.10.0 | MIT |
| `github.com/ledongthuc/pdf` | v0.0.0-20250511090121 | BSD-3-Clause |
| `github.com/prometheus/client_golang` | v1.24.1 | Apache-2.0 |

Transitive dependencies are pulled in through the three above. They end up in
the compiled binary, so they are listed rather than waved away:

| Module | License |
| --- | --- |
| `github.com/beorn7/perks` | MIT |
| `github.com/cespare/xxhash/v2` | MIT |
| `github.com/jackc/pgpassfile` | MIT |
| `github.com/jackc/pgservicefile` | MIT |
| `github.com/jackc/puddle/v2` | MIT |
| `github.com/munnerz/goautoneg` | BSD-3-Clause |
| `github.com/prometheus/client_model` | Apache-2.0 |
| `github.com/prometheus/common` | Apache-2.0 |
| `github.com/prometheus/procfs` | Apache-2.0 |
| `golang.org/x/sync`, `golang.org/x/sys`, `golang.org/x/text` | BSD-3-Clause |
| `google.golang.org/protobuf` | BSD-3-Clause |

**There is no copyleft anywhere in the Go tree** — sixteen modules, all
permissive. That is worth stating explicitly, because it means the compiled
binary carries no source-offer obligation of any kind.

`missioncontrol/` and `integrations/` have **zero** external Go dependencies.
Mission Control is standard library only, and that is a design property worth
keeping: a ticket cockpit that cannot be broken by someone else's release.

### Node — `frontend/`

| Package | Version | License |
| --- | --- | --- |
| `react`, `react-dom` | 19.2.8 | MIT |
| `typescript` | 7.0.2 | Apache-2.0 |
| `vite` | 8.2.2 | MIT |
| `vitest` | 4.1.11 | MIT |
| `@vitejs/plugin-react` | 6.1.1 | MIT |
| `@types/react`, `@types/react-dom` | 19.2.x | MIT |

Only `react` and `react-dom` ship to the browser; the rest are build-time.

### Deployed container images

| Image | License | Note |
| --- | --- | --- |
| `postgres:17-alpine` | PostgreSQL License (BSD-style) | |
| `prom/prometheus:v3.13.0-distroless` | Apache-2.0 | |
| `registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.18.0` | Apache-2.0 | |
| `grafana/grafana:13.2.0` | **AGPL-3.0-only** | see below |

## The two licenses that need an actual argument

Everything above except two entries is permissive and raises no question.

### Grafana is AGPL-3.0

AGPL obligations attach to **conveying** the software and to offering a
**modified** version to users over a network. HomeFlow does neither: Grafana
runs as the unmodified upstream image, on a private laptop, reachable only
through an explicit loopback port-forward, for the two people who own the
machine. No copy is distributed and no source is modified, so no source-offer
obligation arises.

What follows as a rule rather than a footnote: **do not patch or rebuild the
Grafana image.** Provisioning through configuration files and dashboards is
fine — that is data, not a modified work. If Grafana ever needed a patch, the
AGPL question would become real, and that is a decision for Marco, not a
build step.

### Poppler is GPL-2.0/GPL-3.0 — and is not deployed

[ADR 0009](../adr/0009-ocr-engine.md) evaluated a local OCR path using
Tesseract (Apache-2.0, with `tessdata_fast` models also Apache-2.0) and
`pdftoppm` from poppler, which is GPL-licensed.

Poppler is invoked as a **subprocess**, never linked, so no HomeFlow code
becomes a derivative work. The benchmark image under `docker/ocr/` is a local
evaluation artifact and **is not deployed and not published**. Should an OCR
image ever be distributed, the GPL source-offer obligation for poppler applies
to that image and must be handled before, not after.

## What is deliberately absent

No hosted service, no SaaS dashboard, no coverage service, no model API, no
font or icon set with an attribution or registration requirement. Fonts, icons,
datasets and map data count toward rule 2 exactly like code does — the project
uses none that carry conditions.

## Keeping this honest

A license inventory that is written once is wrong within a month. The direct
dependencies listed here are therefore checked against the actual manifests by
`scripts/check-licenses.sh`, which runs inside `make ci`:

- every direct requirement in the three `go.mod` files
- every entry in `frontend/package.json`
- every `image:` in the Kubernetes and monitoring manifests

If any of them is missing from this file, the build fails and names it. The
check verifies that a dependency **has been assessed**, not what its license
says — no tool can decide whether an AGPL component is acceptable in a given
deployment. That judgement is the point of the two sections above.
