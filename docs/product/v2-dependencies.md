# HomeFlow v2: what depends on what

Reading aid for epic HF-0114. An arrow means **"needs"** — the target cannot
start before the source is done.

The same graph exists as a comment on HF-0114; this file is the version that
survives the ticket board.

```mermaid
graph TD
    subgraph V20["V2.0 — Zuschnitt"]
        A116["HF-0116<br/>kein Datensatz ohne Tier"]
        A129["HF-0129<br/>UI-Rahmen, 6 Punkte"]
    end

    subgraph V21["V2.1 — der Besuch"]
        A117["HF-0117<br/>Tierarzt als Entität"]
        A118["HF-0118<br/>MedicalEncounter"]
        A119["HF-0119<br/>Timeline: eine Zeile"]
        A131["HF-0131<br/>UI Tierprofil"]
    end

    subgraph V22["V2.2 — Dokumente"]
        A120["HF-0120<br/>Dokumentenzentrale"]
    end

    subgraph V23["V2.3 — Kopplung"]
        A121["HF-0121<br/>Outbox + Idempotenz"]
    end

    subgraph V24["V2.4 — Geld"]
        A122["HF-0122<br/>Ausgabe aus Beleg"]
    end

    subgraph V25["V2.5 — OCR"]
        A124["HF-0124<br/>Job-Queue"]
        A123["HF-0123<br/>Worker-Pod"]
        A125["HF-0125<br/>Extraktion + Confidence"]
        A126["HF-0126<br/>Prüfen + Bestätigen"]
        A132["HF-0132<br/>UI Prüfschirm"]
    end

    subgraph Rest["V2.6 bis V2.8"]
        A127["HF-0127<br/>Versicherung"]
        A128["HF-0128<br/>Einkauf + Bestand"]
        A134["HF-0134<br/>Erinnerungen"]
        A130["HF-0130<br/>UI Dashboard"]
        A133["HF-0133<br/>UI übrige Bereiche"]
    end

    A116 --> A117 --> A118 --> A119 --> A131
    A118 --> A120 --> A121 --> A122
    A129 --> A131
    A120 --> A124 --> A123 --> A125 --> A126 --> A132
    A122 --> A126
    A122 --> A127
    A118 --> A134
    A116 --> A128
    A126 --> A130
    A127 --> A133
    A128 --> A133

    classDef ready fill:#1d4b32,stroke:#2e7d52,color:#fff
    class A116,A129 ready
```

Ohne Mermaid-Darstellung, dieselbe Aussage:

    HF-0116 Zuschnitt ────┬─► HF-0117 Tierarzt ──► HF-0118 Encounter ──► HF-0119 Timeline
                          │                              │                      │
    HF-0129 UI-Rahmen ────┴──────────────────────────────┼──────────────► HF-0131 UI Tier
                                                         │
                                                         ├─► HF-0134 Erinnerungen
                                                         │
                                                         └─► HF-0120 Dokumente
                                                                  │
                                          ┌───────────────────────┴───────────┐
                                          ▼                                   ▼
                                    HF-0121 Outbox                      HF-0124 Queue
                                          │                                   │
                                          ▼                                   ▼
                                    HF-0122 Ausgaben ──┬──────────────► HF-0123 Worker
                                          │            │                      │
                                          │            └─► HF-0127 Versicherung│
                                          │                        │          ▼
                                          │                        │   HF-0125 Extraktion
                                          │                        │          │
                                          │                        │          ▼
                                          │                        │   HF-0126 Bestätigen
                                          │                        │       │      │
                                          │                        ▼       ▼      ▼
    HF-0116 ─────────────────────► HF-0128 Einkauf ──────► HF-0133 UI  HF-0130 UI  HF-0132 UI

## Green means startable

**HF-0116** and **HF-0129** depend on nothing, touch no shared path
(`backend/internal/{expenses,calendar,shopping}` against `frontend/src/`) and can
run in parallel. Everything else waits on one of them.

## The two gates are open

Both were Marco's to decide and both were approved on 2026-09-01:

- **HF-0120** — the interface change `/api/v1/receipts` → `/api/v1/documents`,
  recorded in `../architecture/contracts.md` section 1.1.
- **HF-0123** — 256 MiB for the OCR worker, recorded in section 2.1. Requests
  rise from 672 to 768 MiB of 3444 MiB allocatable.

## Where the graph will bite

Path claims overlap, because a module gets touched more than once:

| Path | Tickets |
| --- | --- |
| `backend/internal/pets/` | HF-0117, 0118, 0119, 0128, 0134 |
| `backend/internal/expenses/` | HF-0116, 0122 |
| `backend/internal/shopping/` | HF-0116, 0128 |
| `frontend/src/` | HF-0129, 0130, 0131, 0132, 0133 |

That is not a planning error, it is what a modular monolith looks like when a
release series walks through it. The rule is **one at a time, not in parallel**,
and Mission Control's repository view reports the collision the moment two of
them sit in "In Bearbeitung". That view exists for exactly this.

The frontend row is the sharpest one: five tickets on one directory. They are
strictly sequential, and HF-0129 has to be finished before any of the other
four starts — it decides the file layout they all build in.
