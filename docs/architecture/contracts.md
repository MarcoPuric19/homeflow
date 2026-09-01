# Verträge: Schnittstellen, Mengen und Budgets

Dieses Dokument ist der **verbindliche Teil** der früheren `Austausch.md`. Es
steht hier und nicht im Ticketsystem, weil es keine Vorgänge beschreibt,
sondern Zusagen: Dinge, die dauerhaft gelten und mehrere Konsumenten haben. Ein
Ticket schließt man; eine Zahl in diesem Dokument gilt weiter.

**Regel:** Wer etwas hieraus ändert, ändert es in **allen** Konsumenten im
selben Durchgang und hält es in dem Ticket fest, unter dem er arbeitet. Bei
Mengen und Budgets (Abschnitt 2) entscheidet Marco, nicht der Assistent.

Übernommen am 2026-09-01 aus `Austausch.md`, Abschnitte 4 und 5, inhaltlich
unverändert. Verweise auf frühere Beobachtungsnummern zeigen jetzt auf die
entsprechenden Tickets.

---

## 1. Schnittstellen: was nur abgestimmt geändert wird

Diese Dinge haben mehr als einen Konsumenten. Wer eines davon ändert, ändert es
**in allen Konsumenten im selben Durchgang** und trägt es in ein Ticket.

### 1.1 HTTP-API

Vertrag: `docs/api/openapi.yaml` (aktuell `version: 0.15.11`).
Konsumenten: `backend/internal/httpapi/server.go`, `frontend/src/api.ts`,
die `curl`-Beispiele in `README.md`, Tests in `backend/internal/httpapi/*_test.go`.

Die OpenAPI ist die Referenz, nicht das Protokoll des Codes. Neue oder
geänderte Routen zuerst dort, dann implementieren.

Aktueller Stand: `/health`, `/metrics`, `/api/v1/auth/{login,refresh,logout}`,
`/api/v1/auth/browser/{login,refresh,logout}`, `/api/v1/shopping/items[/{id}/complete]`,
`/api/v1/expenses[/{id}/paid]`, `/api/v1/expense-summaries/{month}`,
`/api/v1/expenses/{id}/receipts`, `/api/v1/expenses/{id}/receipts/{receiptId}/content`
(seit 2026-08-31, Download), `/api/v1/calendar/events` (GET mit Pflichtparametern
`from`/`to`, POST) und `/api/v1/calendar/events/{eventId}` (GET, PUT, DELETE — seit
2026-08-31), `/api/v1/pets` (GET mit begrenzter Keyset-Paginierung, POST) und
`/api/v1/pets/{petId}` (GET, PATCH — seit 2026-09-01). Seit Version 0.15.1
tragen Pet-Stammdaten optional ungefähres Geburtsdatum, Farbe/Zeichnung,
Versicherung, Versicherungsnummer und Stammtierarzt. Pet-Zugriffe sind über
die Mitgliedschaft auf den Haushalt begrenzt. Seit 2026-09-01 zusätzlich
`GET/POST /api/v1/pets/{petId}/medical-records` als begrenzte chronologische
Patientenakte.
Seit 2026-09-01 außerdem `GET/POST /api/v1/pets/{petId}/vaccinations` mit
manuell gepflegtem nächsten Fälligkeitsdatum. Seit Version 0.15.2 optional mit
Hersteller, höchstens 20 dokumentierten Krankheiten, manuellem Gültigkeitsdatum
und einem Dokument desselben Haushalts und Tiers.
Kalenderabfragen akzeptieren seit 2026-09-01 optional `petId`; Event-Ein- und
Ausgaben tragen optional `petId` plus `appointmentType`.
Seit 2026-09-01 außerdem `GET/POST /api/v1/pets/{petId}/medications` für
Benutzerdokumentation und exakten, nicht automatisch berechneten Bestand.
Seit 2026-09-01 liefert `GET /api/v1/shopping/lists` die sichtbaren Listen;
Shopping-Items akzeptieren/liefern `petIds`, die Listenabfrage akzeptiert
`petId` und `limit`.
Seit 2026-09-01 sind Ausgaben haushaltsgebunden und akzeptieren optional
`petId` und `budgetCategory`; Listen sind auf 100 begrenzt und nach Tier
filterbar. `GET/PUT /api/v1/pets/{petId}/budgets` pflegt Monatsbudgets je
Kategorie, `GET /api/v1/pets/{petId}/expense-summary?period=YYYY[-MM]` liefert
auf 100 Kategorien begrenzte Monats- oder Jahreswerte.
Seit 2026-09-01 außerdem `GET/POST /api/v1/pets/{petId}/documents` (maximal
100 Metadatensätze, Upload JPEG/PNG/PDF bis 10 MiB) sowie
`GET /api/v1/pets/{petId}/documents/{documentId}/content` als erzwungener,
haushaltsgeprüfter Attachment-Download.
Seit 2026-09-01 außerdem `GET/POST /api/v1/pets/{petId}/weights` und
`GET/POST /api/v1/pets/{petId}/lab-results` (je maximal 100 exakte Messwerte)
sowie `GET /api/v1/reminders?until=…` für höchstens 366 Tage/100 abgeleitete
Impffälligkeits- und Tierterminhinweise ohne Hintergrundzustellung.
Seit 2026-09-01 liefert `GET /api/v1/pets/{petId}/dashboard?month=YYYY-MM`
eine haushaltsgeprüfte, kompakte Tierübersicht in einem Datenbank-Roundtrip.
Seit Version 0.15.3 lesen und ersetzen `GET/PUT
/api/v1/pets/{petId}/documents/{documentId}/links` sowie `GET/PUT
/api/v1/expenses/{expenseId}/pet-links` höchstens je 100 Beziehungen zu
Patientenakten und Tierterminen. PostgreSQL erzwingt denselben Haushalt und
dasselbe Tier; Belege übernehmen Beziehungen ausschließlich über ihre Ausgabe.
Seit Version 0.15.4 liefert `GET /api/v1/pets/{petId}/timeline` höchstens 100
chronologische Einträge aus Patientenakte, Impfungen, Medikamenten, Gewicht,
Labor, Dokumenten und Tierterminen mit Keyset-Paginierung (Standard 30).
Seit Version 0.15.5 lesen und ersetzen `GET/PUT /api/v1/reminder-rules` die
lokalen Vorlaufstufen des Haushalts (Standard 30, 7 und 1 Tage). Reminder tragen
die konkrete Stufe und werden weiterhin ausschließlich beim Lesen berechnet.
Seit Version 0.15.6 erzeugt `POST
/api/v1/reminders/vaccinations/{sourceId}/appointment` auf ausdrückliche
Benutzeraktion höchstens einen Tiertermin je Impfquelle; Eigentümer,
Sichtbarkeit und Zeit werden explizit gesetzt.
Seit Version 0.15.7 tragen Pet-Medikamente optional exakte Packungsgröße und
Mindestbestand in derselben Bestandseinheit sowie manuelle Kauf- und
Haltbarkeitsdaten. Es findet keine Verbrauchsberechnung statt.
Seit Version 0.15.8 bieten Restock-Vorschau und -Bestätigung einen manuellen,
race-sicher idempotenten Übergang von niedrigem Medikamentenbestand zu genau
einem offenen, tierverknüpften Einkaufsartikel. Bestand wird separat manuell
ersetzt; nach Abschluss darf erneut ein Artikel entstehen.
Seit Version 0.15.9 tragen Einkaufsartikel optional eine bevorzugte Marke und
einen Schätzpreis in Integer-Cents plus Währung sowie ein dokumentiertes
Wiederholungskennzeichen. Wiederholung erzeugt niemals automatisch Artikel.
Seit Version 0.15.10 tragen Pet-Monatsbudgets optional eine feste lokale
Warnschwelle in Integer-Cents sowie Notizen. Monats- und Jahresauswertungen
liefern das verbleibende Budget und den serverseitig berechneten Warnzustand;
maßgeblich sind tatsächlich bezahlte Ausgaben, nicht offene Planwerte.

### 1.2 Metriknamen

Definiert in `backend/internal/observability/metrics.go`, konsumiert von den
Grafana-Dashboards in `monitoring/kubernetes/grafana.yaml` und den Scrape-Jobs
in `monitoring/kubernetes/prometheus.yaml`.

```
homeflow_http_requests_total{route,method,status}
homeflow_http_request_duration_seconds{route,method}
homeflow_db_query_duration_seconds{operation,outcome}
homeflow_login_failures_total
homeflow_shopping_items_created_total
homeflow_expenses_created_total
homeflow_receipt_processing_duration_seconds
homeflow_db_pool_acquired_connections
homeflow_db_pool_idle_connections
homeflow_expenses_due
```

Ein umbenannter Zähler bricht ein Dashboard still — die Panels laufen weiter,
zeigen aber nichts. Umbenennungen deshalb nie ohne Anpassung von `grafana.yaml`.
**Seit 2026-08-31 hängen auch die Alarmregeln** (`alerts.yml` in der ConfigMap
`prometheus-config`) an genau diesen Namen — eine Umbenennung deaktiviert dort
still einen Alarm. Sieben der zehn Metriken sind alarmiert; nicht alarmiert sind
`homeflow_db_pool_idle_connections`, `homeflow_shopping_items_created_total`
und `homeflow_expenses_created_total`.

Das Label `route` kommt aus `r.Pattern` (Go-1.22-Routenmuster). Seit 2026-09-01
kopiert die Auth-Middleware das vom inneren Mux getroffene Muster zurück an die
äußere Metrikschicht. Damit erscheinen auch authentifizierte Routen einzeln,
z. B. `GET /api/v1/shopping/items`; dynamische IDs und Benutzereingaben werden
nicht zu Labels. Nicht-gematchte Requests laufen unter `route="unmatched"`.

### 1.3 Datenbank-Migrationen

`backend/migrations/`, aktuell bis `000002_user_limit`. **Die häufigste stille
Kollision:** zwei parallel angelegte `000003_*` sind erst beim Merge sichtbar.
Wer eine Migration plant, trägt die reservierte Nummer sofort hier ein — auch
vor dem Schreiben der Datei.

Vergeben: `000003_households_and_pets` (Codex, Pet E1)
Vergeben: `000004_pet_medical_records` (Codex, Pet E2)
Vergeben: `000005_pet_vaccinations` (Codex, Pet E3)
Vergeben: `000006_pet_calendar_events` (Codex, Pet E4)
Vergeben: `000007_pet_medications` (Codex, Pet E5)
Vergeben: `000008_pet_shopping` (Codex, Pet E6)
Vergeben: `000009_pet_expenses_budgets` (Codex, Pet E7)
Vergeben: `000010_pet_documents` (Codex, Pet E8)
Vergeben: `000011_pet_measurements_reminders` (Codex, Pet E9)
Vergeben: `000012_pet_profile_details` (Codex, HF-0061)
Vergeben: `000013_pet_vaccination_details` (Codex, HF-0062)
Vergeben: `000014_medical_financial_links` (Codex, HF-0063)
Vergeben: `000015_pet_reminder_rules` (Codex, HF-0065)
Vergeben: `000016_reminder_appointments` (Codex, HF-0066)
Vergeben: `000017_pet_medication_inventory` (Codex, HF-0067)
Vergeben: `000018_medication_shopping_origin` (Codex, HF-0068)
Vergeben: `000019_pet_shopping_metadata` (Codex, HF-0070)
Vergeben: `000020_pet_budget_warning_threshold` (Codex, HF-0069)

Weitere Reservierungen: *(noch nichts)*

Migrationen laufen beim API-Start unter einem PostgreSQL Advisory Lock. Sie
sind eingebettet (`migrations/embed.go`), d.h. eine neue Datei wirkt erst nach
einem Rebuild.

### 1.4 Konfiguration (ENV)

Vier Konsumenten, die auseinanderlaufen können: `.env.example`, `compose.yaml`,
`kubernetes/base/configmap.yaml` + `secret.example.yaml`, und das Auslesen in
`backend/cmd/homeflow/main.go`.

Eine neue Variable wird in allen vieren ergänzt, sonst startet entweder Compose
oder Kubernetes nicht. `HOMEFLOW_ENV` akzeptiert bewusst nur `development` und
`local` — das ist die Bremse gegen versehentlichen Prod-Betrieb. **Offener
Vorschlag F2** (Skalierung auf 10.000 Benutzer) würde genau diese Bremse lösen;
sie fällt nur mit einem ADR, nicht nebenbei.

### 1.5 Cookie- und Token-Namen

`homeflow_access`, `homeflow_refresh` (Pfad `/api/v1/auth/browser`),
`homeflow_csrf` (nicht HttpOnly, absichtlich lesbar), Header `X-CSRF-Token`.
Gemeinsam genutzt von `server.go` und `frontend/src/api.ts`. Ein Name, der nur
auf einer Seite geändert wird, äußert sich als stiller 403.

### 1.6 Image-Version

`VERSION` im `Makefile` (aktuell `0.15.9`), das `image:`-Tag in
`kubernetes/base/api-deployment.yaml` und `info.version` in `openapi.yaml`
müssen zusammenpassen. `make deploy` lädt das Image direkt in den kind-Node;
ein Versions-Mismatch zeigt sich als „läuft, aber alter Code".

Versionsregel seit 2026-09-01: Ein normales Ticket erhöht ausschließlich die
Patch-Stelle (höchstens `.100`). Die Umsetzung eines Epics erhöht die mittlere
Stelle und setzt Patch auf `.0`. Das bloße Anlegen eines Epics ändert keine
Version. Jede Zielversion wird vor Arbeitsbeginn im Mission-Control-Ticket
reserviert.

### 1.7 Sprachkonvention

Das Repo ist zweisprachig, aber nicht beliebig. Beim Ergänzen gilt die Sprache
der Datei, nicht die eigene Vorliebe:

| Ort | Sprache |
| --- | --- |
| `README.md` (Wurzel), `Austausch.md` | Deutsch |
| `docs/**` (ADRs, Architektur, Threat Model) | Englisch |
| `monitoring/README.md`, `helm/`, `terraform/`, `worker/` | Englisch |
| Go-/TypeScript-Code, Kommentare, Metrik-`Help` | Englisch |
| Benutzeroberfläche, Alarm-Annotationen | Deutsch (Marco ist der einzige Leser) |

---

## 2. Mengen und Budgets

Das Ganze läuft auf **einem Laptop**. Ressourcen sind hier ein
Korrektheitskriterium, kein Tuning-Thema. Wer eine Zahl ändert, ändert sie hier
mit und prüft die Summe.

### 2.1 RAM-/CPU-Budget

| Komponente | Request | Limit | Gemessene Spitze | Quelle |
| --- | --- | --- | --- | --- |
| API (inkl. statischem Web-UI) | 160 MiB / 10 mCPU | 192 MiB / 200 mCPU | 142 MiB | `kubernetes/base/api-deployment.yaml` |
| PostgreSQL | 128 MiB / 25 mCPU | 384 MiB / 500 mCPU | 47 MiB | `kubernetes/base/postgres.yaml`, `compose.yaml` |
| Prometheus | 160 MiB / 50 mCPU | 384 MiB / 300 mCPU | 133 MiB | `monitoring/kubernetes/prometheus.yaml` |
| Grafana | 160 MiB / 20 mCPU | 256 MiB / 200 mCPU | 165 MiB | `monitoring/kubernetes/grafana.yaml` |
| kube-state-metrics | 64 MiB / 10 mCPU | 128 MiB / 100 mCPU | 63 MiB | `monitoring/kubernetes/kube-state-metrics.yaml` |
| **Monitoring gesamt** | **384 MiB / 80 mCPU** | **768 MiB / 600 mCPU** | 361 MiB | `monitoring/README.md` |

Die Spalte „gemessene Spitze" stammt aus dem cAdvisor-Scrape und ist mit
`scripts/resource-report.sh` reproduzierbar; Herleitung und Optionen stehen in
`docs/operations/resource-budget.md`.

**Requests sind auf diesem Cluster kein Scheduling-Detail.** Der kind-Knoten
meldet `allocatable` = `capacity` und hat keine `evictionHard.memory.available`,
die Speicher-Eviction des kubelets ist also aus. Bei Speichermangel entscheidet
der Host-Kernel nach `oom_score_adj`, und der ergibt sich bei Burstable aus
`1000 − 1000 × request / capacity`. Ein Request unter dem echten Verbrauch
schiebt den Pod nach vorn in die Kill-Reihe. Deshalb liegen die Requests oben
über der gemessenen Spitze statt beim Minimum. `kubernetes/kind-cluster.yaml`
enthält die Reservierungen, die das entschärfen — sie wirken erst beim nächsten
Aufbau des Clusters.

Das API-Limit von 192 MiB ist knapp und nicht beliebig. Zwei Zahlen bilden es,
und sie werden leicht verwechselt:

| Zahl | Wert | Was sie bedeutet |
| --- | --- | --- |
| `argonMemory` | 64 MiB | Speicher **je einzelner** Argon2id-Prüfung |
| `passwordConcurrency` | 1 | wie viele Prüfungen **gleichzeitig** laufen dürfen |

Beide stehen in `backend/internal/auth/`. Ihr **Produkt** ist der Speicher, den
die Anwendung im schlimmsten Fall allein für Passwortarbeit belegt — heute
64 MiB von 192 MiB. Wer eine der beiden oder das Limit anfasst, fasst drei
zusammenhängende Zahlen an; `TestPasswordWorkFitsInTheContainerLimit` bewacht
das Produkt und fällt, sobald es nicht mehr passt.

**Nicht zu verwechseln mit `argonParallelism`** (Wert 2). Das ist der Parameter
`p` des Verfahrens, also die Zahl der Rechenbahnen **innerhalb einer** Prüfung.
Er steht im gespeicherten Hash, verändert den Speicherbedarf nicht und darf
nicht angefasst werden, um Speicher zu sparen — das würde jeden vorhandenen Hash
ungültig machen.

`passwordConcurrency` war 2. Zwei gleichzeitige Anmeldungen haben den Pod auf
191,8 MiB von 192 MiB getrieben und der Kernel hat ihn getötet
(`constraint=CONSTRAINT_MEMCG`, HF-0087). Die Schranke umschließt seit HF-0091
nur noch die Argon2-Arbeit selbst und nicht mehr die ganze Anmeldung, sonst
serialisierte sie auch die Datenbankabfragen.

**PostgreSQL-Parameter sind gesetzt, nicht geerbt** (`args` in
`kubernetes/base/postgres.yaml`): `max_connections=20`, `shared_buffers=96MB`,
`effective_cache_size=192MB`, `maintenance_work_mem=32MB`. Ohne sie galten die
Image-Vorgaben — 100 Verbindungen gegen einen Pool von 5 und 4 GiB
`effective_cache_size` auf einem 7-GiB-Laptop. Diese vier Zahlen gehören zum
384-MiB-Limit und werden mit ihm zusammen geändert (HF-0090).

**Konsequenz für neue Komponenten:** jede zusätzliche Dauerkomponente kostet
RAM, das nicht da ist. Ein Alertmanager (~64 MiB) oder ein Log-Stack (Loki +
Promtail, ~200 MiB) sind deshalb keine Selbstverständlichkeit, sondern eine
Entscheidung für Marco — auch wenn sie kostenlos sind. Kostenlos heißt hier
nicht ressourcenfrei.

### 2.2 Weitere harte Grenzen

| Grenze | Wert | Quelle |
| --- | --- | --- |
| DB-Verbindungspool | max. 5 | `backend/internal/postgres/database.go` |
| PostgreSQL `max_connections` | 20 | `kubernetes/base/postgres.yaml` |
| Gleichzeitige Passwortprüfungen | 1, je ~64 MiB | `passwordConcurrency` in `backend/internal/auth/postgres.go` |
| Argon2id-Parameter | m=64 MiB, t=3, p=2 | `backend/internal/auth/password.go` — steht im Hash, Änderung entwertet alle Passwörter |
| Alarmschwelle Container-Speicher | 85 % des Limits, 10 min | `HomeflowContainerMemoryNearLimit` in `monitoring/kubernetes/prometheus.yaml` |
| Beleg-Upload | 10 MiB + 1 MiB Multipart-Overhead | `receipts.MaxUploadBytes`, `server.go` |
| JSON-Request-Body | 64 KiB | `decodeJSON` in `server.go` |
| HTTP-Header | 16 KiB | `main.go` |
| Login-Rate-Limit | 5 Versuche / 15 min je IP+Benutzername | `newLoginRateLimiter` in `server.go` |
| Access-Token | 15 min | ADR 0003 |
| Refresh-Token | 30 Tage, mit Rotation | ADR 0003 |
| Prometheus-Retention | 7 Tage bzw. 750 MiB | `monitoring/kubernetes/prometheus.yaml` |
| PVCs | HomeFlow 3 GiB, Monitoring 2 GiB | `kubernetes/base/`, `monitoring/kubernetes/` |
| Kalender-Abfragezeitraum | max. 366 Tage je Anfrage | `calendar.MaxRangeDays` |
| Kalender-Erinnerung | max. 40320 Minuten (28 Tage) | `maxReminderMinutes` in `calendar/service.go` |
| Kalender-Textfelder | Titel 300, Beschreibung 2000, Kategorie 100, RRULE 500 Zeichen | `calendar/service.go`, Migration `000001` |
| Benutzerkonten | **genau 2**, keine Registrierung | Migration `000002_user_limit` |
| Pet-Listenabfrage | standardmäßig 50, maximal 100, Keyset-Cursor | `pets.Service` |
| Pet-Textfelder | Name 120, Tierart 80, Rasse 120, Farbe 100, Versicherung/Tierarzt 200, Versicherungsnummer 100, Notizen 2000 Zeichen | `pets.Service`, Migrationen `000003`, `000012` |
| Patientenakte | standardmäßig 30, maximal 100 Einträge je Seite; Details max. 20 Schlüssel | `pets.Service`, Migration `000004` |
| Impfverlauf | standardmäßig 30, maximal 100 Einträge je Seite; keine automatische Intervallberechnung | `pets.Service`, Migration `000005` |
| Medikamente je Tier | maximal 100 je Abfrage; Bestand, Packungsgröße und Mindestbestand als exakte Dezimalzeichenfolgen mit max. 3 Nachkommastellen in derselben Einheit | `pets.Service`, Migrationen `000007`, `000017` |
| Ausgabenliste | maximal 100 je Anfrage; optionaler Monats- und Tierfilter | `expenses.Service`, Migration `000009` |
| Tierbudgets/-auswertung | maximal 100 Kategorien; Budget pro Tier/Monat/Kategorie eindeutig; optionale feste Warnschwelle in Integer-Cents, Notiz maximal 2000 Zeichen; Auswertung genau ein Monat oder Jahr | `expenses.Service`, Migrationen `000009`, `000020` |
| Medizinische Dokumente | 10 MiB je Datei; maximal 100 Metadatensätze je Tierabfrage; gemeinsames 1-GiB-Datei-PVC mit Belegen | `documents.Service`, `receipts.LocalStorage`, Migration `000010` |
| Gewicht/Labor | maximal 100 je Tierabfrage; Gewicht max. 3, Laborwerte max. 4 Nachkommastellen als exakte Dezimalstrings | `pets.Service`, Migration `000011` |
| Abgeleitete Erinnerungen | maximal 366 Tage und 100 Treffer je Anfrage; keine Hintergrundzustellung | `reminders.Service` |
| Reminder-Regeln | 1 bis 10 eindeutige Vorlaufwerte, jeweils 0 bis 366 Tage; Standard 30/7/1 | `reminders.Service`, Migration `000015` |
| Pet-Gesamtzeitleiste | standardmäßig 30, maximal 100 Einträge je Seite; Keyset über Datum, Typ und Quell-ID | `pets.Service` |

Die Zwei-Benutzer-Grenze ist in der Datenbank durchgesetzt, nicht nur in der
Anwendung. Sie ist eine Produktentscheidung — nicht „aufbohren", weil ein Test
einen dritten Benutzer bräuchte. **Ausnahme in Vorbereitung:** F2 im Backlog
beschreibt das Ziel von rund 10.000 Benutzern; dort fällt diese Grenze als
Erstes. Sie fällt aber nur zusammen mit Mandantenfähigkeit (F2.2) und einem
ADR — bis dahin gilt der Satz oben unverändert.

Ebenso laptop-gebunden sind die Zahlen darüber: DB-Pool 5, `max_connections=20`,
Argon2 mit 64 MiB bei **einem** Slot und das API-Limit von 192 MiB sind für zwei
Menschen richtig dimensioniert und in F2.1 als Erstes genannt, was bei 500
gleichzeitigen Benutzern bricht. Ein einzelner Slot heißt dort: Anmeldungen
stehen Schlange. Für zwei Menschen kostet das die Dauer eines Hashes, für 500
wäre es der Engpass — die Antwort darauf ist mehr als eine größere Zahl, weil
64 MiB je Prüfung mit der Gleichzeitigkeit mitwachsen.

---
