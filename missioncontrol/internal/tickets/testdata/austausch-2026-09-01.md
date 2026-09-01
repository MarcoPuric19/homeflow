# Austausch: Codex ↔ Claude

Koordinationsdatei für die beiden Assistenten, die HomeFlow betreuen. Sie ist
Arbeitsmittel, keine Architekturdoku — inhaltliche Entscheidungen gehören
weiterhin in `docs/adr/` bzw. `docs/architecture/`.

**Lies diese Datei am Anfang jeder Session, bevor du eine Datei änderst.
Trage deinen Claim ein, bevor du anfängst — nicht danach.**

Verbindlich für beide: Marco entscheidet. Wo hier ein Konflikt auftaucht, den
diese Datei nicht auflöst, wird er gefragt, statt dass einer von uns die
Schnittstelle einseitig ändert.

## Goldene Regeln

1. **Alles wird hier dokumentiert.** Was nicht in dieser Datei steht, hat für
   den jeweils anderen nicht stattgefunden. Claim vor der Arbeit, Protokoll
   nach der Arbeit, Schnittstellen- und Mengenänderungen im selben Durchgang.
2. **Nur kostenlos.** Keine kostenpflichtigen Clouds, APIs, SaaS-Dienste,
   Accounts, Lizenzen oder Free-Tiers-mit-Kreditkarte. Ausschließlich Open
   Source, lokal betrieben. Diese Regel hat bereits Alexa und Ring
   ausgeschlossen (ADR 0005) und gilt unverändert für jede neue Abhängigkeit.
   Im Zweifel: nicht einbauen, sondern in Abschnitt 6 notieren und Marco fragen.

---

## 1. Arbeitsmodell: Claim pro Aufgabe

Es gibt keine festen Besitzverhältnisse an Verzeichnissen. Wer eine Aufgabe
beginnt, trägt sie in Abschnitt 3 ein — mit **Datei-Scope**, nicht nur mit
Thema. Der Scope ist die eigentliche Aussage: nur die dort genannten Pfade
sind belegt.

Ablauf:

1. Diese Datei lesen. Überschneidet sich mein Scope mit einem aktiven Claim,
   fange ich nicht an, sondern wähle eine andere Aufgabe oder frage Marco.
2. Zeile in Abschnitt 3 eintragen: Aufgabe, Owner, Datei-Scope, Datum, Status.
3. Arbeiten. Wird der Scope größer als geplant, **erst** die Zeile erweitern.
4. Nach Abschluss: Zeile nach Abschnitt 7 (Protokoll) verschieben, mit einem
   Satz zum Ergebnis. Berührt die Änderung einen Vertrag aus Abschnitt 4 oder
   ein Budget aus Abschnitt 5, dort im selben Zug nachziehen.

Regeln, die Ärger vermeiden:

- **Kein Claim auf `Austausch.md` selbst.** Wer eintragen will, trägt ein.
- **Ein Claim ist kein Freibrief für Nebenwege.** Fällt außerhalb des Scopes
  etwas auf, kommt es in Abschnitt 6 (Beobachtungen), nicht in einen Fix.
- **Verwaiste Claims:** Ist eine Zeile älter als 7 Tage und der Status hat sich
  nicht bewegt, darf der andere sie übernehmen — vorher aber Marco fragen.
- **Formatierungsläufe** (`make fmt`) verschieben ganze Dateibäume und werden
  wie ein Claim behandelt, nicht nebenbei ausgeführt.
- Migrationen und OpenAPI sind die zwei häufigsten stillen Kollisionen
  → siehe Abschnitt 4.

---

## 2. Umgebungen: wer kann was verifizieren

Wir laufen nicht in derselben Umgebung, und das ist für die Aufgabenverteilung
wichtiger als jede Themenaufteilung. **Ungeprüfter Code wird nicht abgeliefert
— wer eine Aufgabe nicht verifizieren kann, nimmt sie nicht.**

| Fähigkeit | Codex (lokal) | Claude Code (lokal) | Claude via Cowork (Cloud-Container) |
| --- | --- | --- | --- |
| Dateien im Repo lesen/schreiben | ja | ja | ja (über die Geräte-Brücke) |
| Shell im Repo | ja | ja | **nein** |
| `go build` / `go test` / `go vet` | ja | ja (Go 1.26.0) | **nein** — Container hat Go 1.24.7, `go.mod` verlangt 1.26, und der Modul-Proxy ist gesperrt |
| `npm test` / `tsc` | ja | ja | **nein** — kein Zugriff auf die npm-Registry |
| `kubectl`, `kustomize`, `kind`, `docker` | ja | ja (Kontext `kind-dev`) | **nein** |
| `promtool` | ? | **nein** — nicht installiert, s. 2.1 | **nein** |
| YAML-/SQL-/Markdown-Prüfung | ja | ja | ja (Python, lokales PostgreSQL) |

Daraus folgt die praktische Regel:

- **Alles unter `backend/` und `frontend/`** braucht jemanden mit Toolchain.
  Claude via Cowork darf dort Spezifikationen und Reviews liefern, aber keine
  fertigen Änderungen — die gehören zu Codex oder zu **Claude Code auf dem
  Laptop**, das dieselbe Toolchain wie Codex hat und dieselbe `Austausch.md`
  liest. Für einen Implementierungs-Loop ist Claude Code der richtige Ort.
- **`kubernetes/`, `monitoring/`, `docs/`, `scripts/`, `README.md`** kann
  Claude via Cowork vollständig verantworten.

Läuft Claude Code auf dem Laptop, gelten für ihn dieselben Claim-Regeln — er
trägt sich als **`Claude Code`** ein, damit im Protokoll unterscheidbar bleibt,
welche Umgebung gearbeitet hat (so steht es auch in `CLAUDE.md`).

### 2.1 Integrationstests ohne Dauer-PostgreSQL

`make integration-test` verlangt `HOMEFLOW_TEST_DATABASE_URL` mit einer
Datenbank, deren Name auf `_test` endet. Wer keine dauerhafte Testdatenbank
betreiben will, nimmt einen Wegwerf-Container — das Image liegt für den
kind-Cluster ohnehin lokal, kostet nichts und ist nach dem Lauf wieder weg:

```bash
PW=$(openssl rand -hex 16)
docker run -d --rm --name homeflow-inttest -e POSTGRES_PASSWORD="$PW" \
  -e POSTGRES_DB=homeflow_test -p 127.0.0.1:15432:5432 postgres:17-alpine
until docker exec homeflow-inttest psql -U postgres -d homeflow_test -c 'select 1' >/dev/null 2>&1; do sleep 1; done
HOMEFLOW_TEST_DATABASE_URL="postgres://postgres:$PW@127.0.0.1:15432/homeflow_test?sslmode=disable" make integration-test
docker rm -f homeflow-inttest
```

`pg_isready` allein reicht als Wartebedingung **nicht**: PostgreSQL meldet sich
schon während der initdb-Phase bereit und startet danach neu, der erste Connect
läuft dann in ein „connection reset by peer". Deshalb oben die `psql`-Schleife.

Dasselbe Muster trägt auch einen vollständigen Ende-zu-Ende-Lauf: eigene
Datenbank, eigene Bootstrap-Konten, eigener Port, `HOMEFLOW_WEB_ROOT` auf
`frontend/dist`. Benutzernamen brauchen dabei mindestens drei Zeichen, sonst
schlägt `users_username_check` beim Bootstrap zu.

**`make ci` enthält die Integrationstests bewusst nicht** — es soll ohne
laufende Datenbank durchlaufen.

---

## 3. Aktive Claims

| Aufgabe | Owner | Datei-Scope | Seit | Status |
| --- | --- | --- | --- | --- |
| Pet-Abschlussaudit: indexfähige Finanzzeiträume und Laborbereich-Validierung | Codex | `backend/internal/expenses/postgres.go`, `backend/internal/pets/{measurement_service.go,postgres.go,service_test.go}`, `backend/internal/integration/postgres_test.go`, `docs/architecture/domain-model.md` | 2026-09-01 | in Arbeit |

*(Der `promtool`-Lauf, der hier offen stand, ist erledigt: `monitoring/README.md`
dokumentiert eine Regeldatei, zehn geprüfte Regeln und vier gesunde Targets, und
der Stack läuft auf dem Cluster. Die Lücke aus dem 2026-08-31-Protokoll ist zu.)*

---

## 4. Schnittstellen: was nur abgestimmt geändert wird

Diese Dinge haben mehr als einen Konsumenten. Wer eines davon ändert, ändert es
**in allen Konsumenten im selben Durchgang** und trägt es in Abschnitt 7 ein.

### 4.1 HTTP-API

Vertrag: `docs/api/openapi.yaml` (aktuell `version: 0.15.0`).
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
`/api/v1/pets/{petId}` (GET, PATCH — seit 2026-09-01). Pet-Zugriffe sind über
die Mitgliedschaft auf den Haushalt begrenzt. Seit 2026-09-01 zusätzlich
`GET/POST /api/v1/pets/{petId}/medical-records` als begrenzte chronologische
Patientenakte.
Seit 2026-09-01 außerdem `GET/POST /api/v1/pets/{petId}/vaccinations` mit
manuell gepflegtem nächsten Fälligkeitsdatum.
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

### 4.2 Metriknamen

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

### 4.3 Datenbank-Migrationen

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

Reserviert: *(noch nichts)*

Migrationen laufen beim API-Start unter einem PostgreSQL Advisory Lock. Sie
sind eingebettet (`migrations/embed.go`), d.h. eine neue Datei wirkt erst nach
einem Rebuild.

### 4.4 Konfiguration (ENV)

Vier Konsumenten, die auseinanderlaufen können: `.env.example`, `compose.yaml`,
`kubernetes/base/configmap.yaml` + `secret.example.yaml`, und das Auslesen in
`backend/cmd/homeflow/main.go`.

Eine neue Variable wird in allen vieren ergänzt, sonst startet entweder Compose
oder Kubernetes nicht. `HOMEFLOW_ENV` akzeptiert bewusst nur `development` und
`local` — das ist die Bremse gegen versehentlichen Prod-Betrieb. **Offener
Vorschlag F2** (Skalierung auf 10.000 Benutzer) würde genau diese Bremse lösen;
sie fällt nur mit einem ADR, nicht nebenbei.

### 4.5 Cookie- und Token-Namen

`homeflow_access`, `homeflow_refresh` (Pfad `/api/v1/auth/browser`),
`homeflow_csrf` (nicht HttpOnly, absichtlich lesbar), Header `X-CSRF-Token`.
Gemeinsam genutzt von `server.go` und `frontend/src/api.ts`. Ein Name, der nur
auf einer Seite geändert wird, äußert sich als stiller 403.

### 4.6 Image-Version

`VERSION` im `Makefile` (aktuell `0.15.0`), das `image:`-Tag in
`kubernetes/base/api-deployment.yaml` und `info.version` in `openapi.yaml`
müssen zusammenpassen. `make deploy` lädt das Image direkt in den kind-Node;
ein Versions-Mismatch zeigt sich als „läuft, aber alter Code".

### 4.7 Sprachkonvention

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

## 5. Mengen und Budgets

Das Ganze läuft auf **einem Laptop**. Ressourcen sind hier ein
Korrektheitskriterium, kein Tuning-Thema. Wer eine Zahl ändert, ändert sie hier
mit und prüft die Summe.

### 5.1 RAM-/CPU-Budget

| Komponente | Request | Limit | Quelle |
| --- | --- | --- | --- |
| API (inkl. statischem Web-UI) | 32 MiB / 10 mCPU | 192 MiB / 200 mCPU | `kubernetes/base/api-deployment.yaml` |
| PostgreSQL | — | 384 MiB (Compose) | `compose.yaml`, `kubernetes/base/postgres.yaml` |
| Monitoring-Stack gesamt | 224 MiB / 80 mCPU | 768 MiB / 600 mCPU | `monitoring/README.md` |

Das API-Limit von 192 MiB ist knapp und nicht beliebig: Argon2id belegt pro
Passwortprüfung rund **64 MiB**, maximal zwei Prüfungen laufen gleichzeitig.
Wer die Argon2-Parameter, die Parallelität oder das Memory-Limit anfasst, fasst
drei zusammenhängende Zahlen an.

**Konsequenz für neue Komponenten:** jede zusätzliche Dauerkomponente kostet
RAM, das nicht da ist. Ein Alertmanager (~64 MiB) oder ein Log-Stack (Loki +
Promtail, ~200 MiB) sind deshalb keine Selbstverständlichkeit, sondern eine
Entscheidung für Marco — auch wenn sie kostenlos sind. Kostenlos heißt hier
nicht ressourcenfrei.

### 5.2 Weitere harte Grenzen

| Grenze | Wert | Quelle |
| --- | --- | --- |
| DB-Verbindungspool | max. 5 | `backend/internal/postgres/database.go` |
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
| Pet-Textfelder | Name 120, Tierart 80, Rasse 120, Notizen 2000 Zeichen | `pets.Service`, Migration `000003` |
| Patientenakte | standardmäßig 30, maximal 100 Einträge je Seite; Details max. 20 Schlüssel | `pets.Service`, Migration `000004` |
| Impfverlauf | standardmäßig 30, maximal 100 Einträge je Seite; keine automatische Intervallberechnung | `pets.Service`, Migration `000005` |
| Medikamente je Tier | maximal 100 je Abfrage; Bestand als exakte Dezimalzeichenfolge mit max. 3 Nachkommastellen | `pets.Service`, Migration `000007` |
| Ausgabenliste | maximal 100 je Anfrage; optionaler Monats- und Tierfilter | `expenses.Service`, Migration `000009` |
| Tierbudgets/-auswertung | maximal 100 Kategorien; Budget pro Tier/Monat/Kategorie eindeutig; Auswertung genau ein Monat oder Jahr | `expenses.Service`, Migration `000009` |
| Medizinische Dokumente | 10 MiB je Datei; maximal 100 Metadatensätze je Tierabfrage; gemeinsames 1-GiB-Datei-PVC mit Belegen | `documents.Service`, `receipts.LocalStorage`, Migration `000010` |
| Gewicht/Labor | maximal 100 je Tierabfrage; Gewicht max. 3, Laborwerte max. 4 Nachkommastellen als exakte Dezimalstrings | `pets.Service`, Migration `000011` |
| Abgeleitete Erinnerungen | maximal 366 Tage und 100 Treffer je Anfrage; keine Hintergrundzustellung | `reminders.Service` |

Die Zwei-Benutzer-Grenze ist in der Datenbank durchgesetzt, nicht nur in der
Anwendung. Sie ist eine Produktentscheidung — nicht „aufbohren", weil ein Test
einen dritten Benutzer bräuchte. **Ausnahme in Vorbereitung:** F2 im Backlog
beschreibt das Ziel von rund 10.000 Benutzern; dort fällt diese Grenze als
Erstes. Sie fällt aber nur zusammen mit Mandantenfähigkeit (F2.2) und einem
ADR — bis dahin gilt der Satz oben unverändert.

Ebenso laptop-gebunden sind die Zahlen darüber: DB-Pool 5, Argon2 mit 64 MiB
bei zwei Slots und das API-Limit von 192 MiB sind für zwei Menschen richtig
dimensioniert und in F2.1 als Erstes genannt, was bei 500 gleichzeitigen
Benutzern bricht.

---

## 6. Backlog

> **Seit 2026-09-01 auch als Tickets.** Die Abschnitte 6, 7 und 8 sind nach
> Mission Control übernommen: 2 Epics, 11 Backlog-Aufgaben, 12 Beobachtungen und
> 28 Protokolleinträge, nachzulesen unter <http://127.0.0.1:7777> im Backlog.
> Diese Datei bleibt bis auf Weiteres die verbindliche Quelle — die Ablösung ist
> Ticket MC-0002 und Marcos Entscheidung. Bis dahin gilt: **hier eintragen, dort
> nachschlagen.** Der Import ist wiederholbar und überspringt, was es schon gibt.


Stand der Lückenanalyse: 2026-08-31, gegen Schema, Code, Manifeste und Doku
geprüft. Priorität ist ein Vorschlag, keine Anweisung — Marco entscheidet.

**Seit 2026-08-31 gilt eine neue Reihenfolge für v1** (von Marco vorgegeben):
Auth → Pets → Patientenakte → Impfungen → Tierarzttermine → Medikamente →
haustierbezogene Einkaufsliste → Budgets/Ausgaben je Tier → Dokumentupload →
Erinnerungen. Das Vorhaben steht als **F1** unten. Die allgemeine
Haushalts-Einkaufsliste ist damit nachrangig — P2-5 bleibt aber wichtig, weil
das Pet-Modul darauf aufsetzt (s. F1.2).
Spalte „Prüfbar durch" folgt aus Abschnitt 2.

### F1 — Pet-Modul (großes Vorhaben, **E1–E10 abgeschlossen**, Stand 2026-09-01)

Marco hat am 2026-08-31 ein Haustier-Modul als künftigen Schwerpunkt von
HomeFlow beauftragt: haustierbezogene Einkaufsliste, Patientenakte, Impfungen,
Tierarzttermine, Medikamente inkl. Bestand, Dokumente, Budgets und Ausgaben je
Tier, Erinnerungen, Verlauf je Tier, mehrere Tiere. **E1 bis E10 (Stammdaten,
Patientenakte, Impfungen, Tiertermine, Medikamente, Einkauf sowie Tierbudgets
und -ausgaben, medizinische Dokumente, Messwerte und abgeleitete Erinnerungen)
sind implementiert**; das geplante Vorhaben ist damit vollständig umgesetzt.
Die allgemeine Haushalts-Einkaufsliste ist damit nachrangig.

Fachliche Grenze, die dauerhaft gilt: HomeFlow speichert, erinnert und stellt
dar. Es stellt **keine** Diagnosen, bestimmt **keine** Dosierungen, trifft
**keine** Impfentscheidungen und empfiehlt **keine** Behandlungen.

**Zusammenhang mit F2:** Marco hat am 2026-08-31 nachgeschoben, dass dies eine
Beta ist und später rund 10.000 registrierte Benutzer bei mindestens 500
gleichzeitigen erreicht werden sollen. Falls das kommt, wird **F2.2
(Mandantenfähigkeit) zur Voraussetzung von F1** — jede Pet-Tabelle bräuchte
sonst nachträglich eine Haushalts-Spalte samt Umbau aller Abfragen. Siehe F2.7.

#### F1.1 Was der Bestand schon hergibt

Gegen Schema und Code geprüft, nicht geschätzt:

- **Der Sicherheits-Pfad für Dokumente existiert fertig.** `receipts`-Upload
  prüft Magic Bytes, erzeugt den Speicherschlüssel serverseitig, begrenzt auf
  10 MiB; der Download (seit P2-2) liefert immer `attachment`, pinnt den
  Content-Type, setzt eine eigene CSP und validiert gegen Traversal. Die
  Sicherheitsanforderungen der Vorlage sind damit **bereits erfüllt** — das
  Dokument-Feature erbt `LocalStorage`, `pinnedMIMEType`,
  `attachmentDisposition` und `id.IsUUID`, statt sie neu zu bauen.
- **Der Kalender kann Tiertermine tragen.** Seit P2-3 gibt es Modell,
  Sichtbarkeitsregel, CRUD, Bereichsabfrage und UI. Ein `PetAppointment` als
  eigene Tabelle würde das alles duplizieren.
- **Die Zwei-Benutzer-Grenze ist nicht betroffen.** Tiere sind keine Benutzer;
  Migration `000002` bleibt unangetastet.
- **Der Modul-Zuschnitt passt.** `internal/<modul>` mit Model, Service,
  Repository, Routen ist viermal erprobt (shopping, expenses, receipts,
  calendar). Das Pet-Modul fügt sich ein, ohne dass etwas umgebaut wird.

#### F1.2 Vier Stellen, an denen der Bestand **nicht** passt

Das sind die eigentlichen Entscheidungen — hier bitte nicht drüberlesen.

1. **`receipts.expense_id` ist `NOT NULL REFERENCES expenses(id)`.** Ein
   Laborbericht, ein Befund oder eine Impfausweis-Seite hat keine Rechnung.
   Entweder wird die Spalte nullable (plus CHECK, dass genau ein Bezug gesetzt
   ist), oder es gibt eine getrennte Tabelle `documents` für medizinische
   Unterlagen. **Empfehlung: getrennte Tabelle** — genau das verlangt die
   Architekturregel der Vorlage („medizinische Daten und Finanzdaten sind
   getrennte Domänen"). `receipts` bleibt dann rein finanziell.
2. **Es gibt nichts, was zu einem Zeitpunkt etwas tut.** Kein Scheduler, kein
   Worker (`worker/` ist ein leerer Platzhalter), kein Alertmanager (RAM,
   Abschnitt 5.1), keine Benachrichtigungsstrecke — weder Mail noch Push.
   `event_outbox` hat weiterhin **keinen Schreiber**. „Reminder automatisch
   erzeugen" ist deshalb in v1 nicht das, wonach es klingt.
   **Empfehlung: Erinnerungen beim Lesen berechnen** (`GET /api/v1/reminders?
   until=…` leitet aus `next_due_date` und Terminen ab, dazu die konfigurierten
   Vorlaufzeiten 30/7/1 Tage). Kostet null zusätzliche Komponenten, null RAM,
   und ist für zwei Menschen, die ohnehin die App öffnen, ausreichend. Echte
   Zustellung wäre eine eigene Entscheidung mit RAM-Preis.
3. **Ein Artikel soll mehreren Tieren zuordenbar sein.** Das ist n:m und geht
   nicht mit einer `pet_id`-Spalte. Nötig ist eine Verknüpfungstabelle
   (`shopping_item_pets`); „allgemeiner Haustierhaushalt" ist dann schlicht ein
   Artikel ohne Zuordnung.
4. **Die Einkaufsliste ist noch nicht erreichbar.** Das Frontend hardcodet die
   UUID der gemeinsamen Liste, es gibt keinen Listen-Endpunkt (P2-5). Sobald
   Haustierbedarf eine eigene Liste bekommen soll, wird P2-5 zur
   **Voraussetzung**, nicht zum Nice-to-have.

#### F1.3 Empfohlener Zuschnitt: 8 Tabellen statt 16

Die Vorlage nennt u. a. `MedicalRecord`, `Diagnosis`, `Treatment`, `Procedure`,
`LabResult`, `WeightEntry`, `MedicalNote`, `Document` als getrennte Entitäten.
Für zwei Menschen und eine Handvoll Tiere wären das sieben fast identische
Tabellen mit je Repository, Service, Routen und Tests — viel Fläche, wenig
Aussage. Die Hauptansicht ist ohnehin **eine chronologische Zeitleiste**.

Vorschlag:

| Tabelle | Zweck | Warum nicht feiner |
| --- | --- | --- |
| `pets` | Stammdaten, `active`-Flag statt Löschen | — |
| `medical_records` | ein Eintrag der Akte mit `record_type` (`diagnosis`, `treatment`, `procedure`, `examination`, `note`, `imaging`, `report`) | eine Zeitleiste, ein Schema; Typ-spezifische Felder in `details jsonb` |
| `lab_results` | Messwerte mit Einheit und Referenzbereich | echte Zahlen, andere Abfragen als Freitext |
| `weight_entries` | Gewichtsverlauf | eigene Kardinalität, wird als Kurve gelesen |
| `vaccinations` | Impfungen inkl. `next_due_date` | — |
| `medications` + `pet_medications` | Präparat und die Gabe je Tier (Dosis, Frequenz, Zeitraum, Bestand) | Bestand als Spalten an `pet_medications`, keine eigene Tabelle in v1 |
| `documents` | medizinische Unterlagen, getrennt von `receipts` | s. F1.2 (1) |
| `pet_budgets` | Budget je Tier, Periode und Kategorie | — |
| `shopping_item_pets` | n:m Artikel ↔ Tier | s. F1.2 (3) |

Dazu **nullable Spalten** an Bestehendem, kein Umbau:
`expenses.pet_id`, `expenses.budget_category`, `calendar_events.pet_id`,
`calendar_events.appointment_type`, `shopping_items.pet_relevant`
(plus die optionalen Felder `preferred_brand`, `estimated_price`, `recurring`).

**Kein `PetAppointment`:** Tiertermine sind `calendar_events` mit `pet_id` und
`appointment_type`. Damit erben sie Bereichsabfrage, CRUD, Sichtbarkeit und UI.

#### F1.4 Etappen

Jede Etappe ist einzeln lauffähig und einzeln verifizierbar. Reihenfolge folgt
Marcos Prioritätsliste; Auth (Punkt 1 seiner Liste) existiert bereits.

| # | Inhalt | Migration | Prüfbar durch |
| --- | --- | --- | --- |
| E1 ✅ | `pets`: Model, Repository, Service, `GET/POST /pets`, `GET/PATCH /pets/{id}`, OpenAPI, Unit- und Integrationstests | `000003` | `make ci` + `make integration-test` |
| E2 ✅ | Akte-Grundstruktur: `medical_records` chronologisch, `GET/POST /pets/{id}/medical-records` | `000004` | dito |
| E3 ✅ | `vaccinations` inkl. `next_due_date` | `000005` | dito |
| E4 ✅ | Termine: `pet_id` + `appointment_type` an `calendar_events`, Filter `?petId=` | `000006` | dito |
| E5 ✅ | `medications` + `pet_medications` inkl. Bestandsfelder | `000007` | dito |
| E6 ✅ | Einkaufsliste: `shopping_item_pets`, Pet-Filter und P2-5 | `000008` | dito |
| E7 ✅ | `expenses.pet_id` + `pet_budgets` + Auswertung je Tier/Kategorie/Monat/Jahr | `000009` | dito |
| E8 ✅ | `documents` inkl. Download über den bestehenden sicheren Pfad | `000010` | dito |
| E9 ✅ | Abgeleitete Erinnerungen (`GET /reminders`), `weight_entries`, `lab_results` | `000011` | dito |
| E10 ✅ | Pet-Dashboard-Endpunkt (eine Abfrage je Tier statt sieben Einzelaufrufe) und UI | 0 | dito |

Migrationsnummern werden **erst beim Start der jeweiligen Etappe** in
Abschnitt 4.3 reserviert — nicht auf Vorrat, sonst blockieren sie Codex.
Grobschätzung: neun Migrationen, rund 25 neue Endpunkte, ein neues UI-Kapitel.

#### F1.5 Was Marco vor dem Start entscheiden muss

1. **Erinnerungen:** beim Lesen berechnen (0 €, 0 MiB) oder echte Zustellung
   mit Scheduler und Kanal (eigene Entscheidung, RAM-relevant)? *Empfehlung:
   berechnen.*
2. **Dokumente:** getrennte Tabelle `documents` (Empfehlung) oder `receipts`
   aufbohren?
3. **Zuschnitt der Akte:** eine `medical_records`-Tabelle mit Typ (Empfehlung)
   oder sieben getrennte Entitäten wie in der Vorlage?
4. **Verzeichnislayout:** die Vorlage schlägt `backend/modules/…` vor, der
   Bestand ist `backend/internal/…`. Ein Umbenennen berührt jede Datei und
   jeden Claim. *Empfehlung: lassen.*
5. **`event_outbox`:** endlich einen Schreiber bekommen (die Vorlage nennt 17
   Domain-Events) oder weiter liegen lassen, bis es einen Konsumenten gibt?
   *Empfehlung: liegen lassen* — eine Tabelle voller ungelesener Zeilen ist
   kein Fortschritt.

#### F1.6 Ressourcen und Kosten

- **Kosten: 0 €.** Alles läuft lokal: PostgreSQL, das bestehende Binary, das
  bestehende PVC. Keine OCR, keine Tierarzt-Schnittstelle, kein Cloud-Dienst,
  keine kostenpflichtige API — die Regel bleibt unverletzt. Die einzige
  Position, die je Geld kosten könnte, wäre ein externer Impf- oder
  Medikamentenkatalog; **nicht vorgesehen**.
- **RAM: unverändert**, solange Erinnerungen berechnet statt zugestellt werden.
  Kein neuer Prozess, kein neuer Container. Der Speicherbedarf des Binaries
  wächst um Handlerflächen, nicht um Datenhaltung.
- **Datenträger: die einzige echte Größe.** Stammdaten und Akte sind winzig;
  **Dokumente dominieren.** Korrektur zu einer früheren Fassung dieses Absatzes:
  die 3 GiB aus Abschnitt 5.2 sind die **Summe** aus PostgreSQL (2 GiB) und
  Belegen (1 GiB) — Dokumente landen auf dem Beleg-PVC, also auf **1 GiB**. Bei
  10 MiB Upload-Limit sind das rund **100** Dateien in Maximalgröße, realistisch
  einige hundert Scans. Vor E8 gehört deshalb geklärt, ob das PVC wächst und ob
  es eine Aufbewahrungsregel gibt.
- **Datenbank:** einige tausend Zeilen über Jahre. Der Pool bleibt bei 5.
- **Metriken:** keine neuen Namen nötig. **Aber:** die rund 25 neuen Routen
  landen alle unter `route="/api/"` (Beobachtung 7.7) — vor dem Pet-Modul wäre
  das ein guter Zeitpunkt, das zu beheben, sonst wird die Kennzahl endgültig
  nichtssagend.

#### F1.7 Ausdrücklich nicht Teil von v1

Automatische medizinische Entscheidungen jeder Art, OCR, Smart-Home-Anbindung
für Tierdaten, externe Tierarzt-Schnittstellen, Cloud, kostenpflichtige APIs,
hart kodierte Impfintervalle (das nächste Fälligkeitsdatum wird **manuell**
eingetragen), automatische Verbrauchsberechnung für Medikamente.

### F2 — Skalierung auf 10.000 Benutzer (Beta-Ziel, **nicht begonnen**)

Marco hat am 2026-08-31 nachgeschoben: der jetzige Stand ist eine Beta,
künftig sollen rund **10.000 registrierte** Benutzer bei mindestens **500
gleichzeitig aktiven** möglich sein, und das Backend soll entsprechend
skalierbar ausgelegt werden. Zusätzlich soll die Anwendung „so komfortabel und
intuitiv wie möglich" sein (F2.6).

**Das steht im Widerspruch zu drei Festlegungen, die dieses Repo tragen**, und
keine davon löse ich einseitig auf:

1. **Die Zwei-Benutzer-Grenze** ist in der Datenbank durchgesetzt (Trigger in
   Migration `000002`) und laut `AGENTS.md` nichts, was im Alleingang fällt.
2. **Kein Remote Access.** `HOMEFLOW_ENV` akzeptiert nur `development` und
   `local`, `HOMEFLOW_COOKIE_SECURE=false`, kein Ingress, kein TLS, keine
   Zertifikate — die Gates dafür stehen im Threat Model und sind nicht erfüllt.
3. **Die 0-Euro-Regel.** 10.000 Benutzer laufen nicht auf einem Laptop. Marco
   hat diese Regel eine Nachricht zuvor ausdrücklich bestätigt; F2.5 benennt,
   was sie in diesem Szenario kostet.

Dieser Eintrag ist deshalb ein **Zielbild mit Preisschild**, keine begonnene
Arbeit. Nichts davon ist implementiert.

#### F2.1 Was bei 500 gleichzeitigen Benutzern zuerst bricht

Am Code nachgesehen, nicht geschätzt:

| Stelle | heute | Wirkung bei 500 gleichzeitig |
| --- | --- | --- |
| Benutzer-Trigger, Migration `000002` | genau 2 Konten | der dritte `INSERT` scheitert — harte Sperre |
| Argon2id, `argonMemory = 64 MiB` | 64 MiB je Prüfung, **2 Slots** (`passwordSlots`) | zwei parallele Anmeldungen, alles andere wartet; eine Anmeldewelle wird zur Warteschlange |
| API-Limit 192 MiB | 2 × 64 MiB Argon2 + Laufzeit | keine Reserve für Replikate mit mehr Parallelität |
| DB-Pool `MaxConns = 5` | je Replikat 5 | **jede** authentifizierte Anfrage macht mindestens einen Session-Lookup → Sättigung lange vor 500 |
| Sessions in der Datenbank | Token-Hash-Lookup pro Request | die Obergrenze des Durchsatzes, nicht die CPU |
| Login-Rate-Limit | `map` im Prozessspeicher, **ohne Eviction** | pro Replikat ein eigener Zustand (wirkungslos), und die Map wächst unbegrenzt — bei fremden Benutzernamen aus dem Netz ein Speicherleck |
| Belege auf PVC, `ReadWriteOnce`, 1 GiB | ein Pod kann schreiben | **blockiert horizontale Skalierung**; 1 GiB reicht für ~100 Dateien |
| `homeflow_expenses_due` | `SELECT count(*)` bei **jedem** Scrape (15 s) | Full Scan über alle Mandanten, alle 15 Sekunden |
| `route`-Label | alles unter `route="/api/"` (7.7) | bei 500 Benutzern ist die Kennzahl blind |
| Registrierung | existiert nicht, bewusst | 10.000 Benutzer brauchen Selbstregistrierung, Verifikation, Passwort-Reset |

#### F2.2 Die eine Änderung, die alles andere bestimmt: Mandantenfähigkeit

**Heute ist die Datenbank der Haushalt.** Es gibt kein Haushalts- oder
Mandantenkonzept — und genau das macht den Schritt gefährlich, nicht nur groß:

- Der Kalender filtert mit `visibility = 'shared' OR owner_id = $1`. Mit 10.000
  Benutzern in derselben Tabelle bedeutet `shared` **„für alle sichtbar"** —
  jeder geteilte Termin läge offen. Dieselbe Zeile, die heute korrekt ist, ist
  dann ein Datenleck.
- Die Einkaufsliste hängt an **einer** fest verdrahteten UUID
  (`shopping.SharedListID`), die beim Bootstrap angelegt wird.
- Ausgaben und Belege haben überhaupt keine Zuordnung außer `user_id`.

Nötig ist deshalb **vor allem anderen**: eine `households`-Tabelle,
`household_id` als Pflichtspalte auf jeder fachlichen Tabelle, Filterung in
jeder Abfrage, abgesichert durch **Row Level Security in PostgreSQL** als
zweite Verteidigungslinie — die Anwendung darf nicht die einzige Stelle sein,
die den Mandanten kennt. Dazu je Modul ein Test, dass ein fremder Haushalt 404
bekommt, analog zu dem, was der Kalender heute für private Termine tut.

**Konsequenz für F1:** wenn das Pet-Modul kommt, gehört `household_id` von der
ersten Migration an hinein — auch solange es nur einen Haushalt gibt.
Nachrüsten heißt später: neun Tabellen, jede Abfrage, jeder Test.

#### F2.3 Zielarchitektur — weiterhin ein modularer Monolith

Skalierung heißt hier **nicht** Microservices. Der Monolith wird zustandslos
und N-fach gestartet:

- **Zustandslose API.** Nichts Relevantes im Prozessspeicher: Rate-Limit in
  einen gemeinsamen Speicher (PostgreSQL reicht anfangs; Valkey/Redis, falls
  es klemmt) oder an den Ingress.
- **Auth-Kosten senken.** Entweder kurzlebige signierte Access-Tokens ohne
  DB-Lookup je Anfrage (Refresh bleibt in der Datenbank, Widerruf über eine
  kurze Sperrliste), oder ein Session-Cache. Das ist der größte Hebel für
  Durchsatz.
- **Argon2 neu parametrisieren.** 64 MiB je Prüfung sind für einen Laptop mit
  zwei Menschen richtig und für 500 gleichzeitige Benutzer nicht bezahlbar.
  OWASP-konforme Alternative: ~19 MiB bei höherer Iterationszahl, plus
  ausdrückliche Parallelitätsgrenze. **Drei zusammenhängende Zahlen** —
  Speicher, Parallelität, Pod-Limit — wie in Abschnitt 5.1 beschrieben.
- **Dateien in einen Objektspeicher.** `ReadWriteOnce` ist mit mehreren
  Replikaten nicht haltbar. MinIO ist S3-kompatibel, Open Source und
  selbst betreibbar; der bestehende sichere Download-Pfad (Attachment,
  Content-Type-Pinning, serverseitiger Schlüssel) bleibt dabei erhalten und
  wird nur auf ein anderes Backend gelegt.
- **Datenbank.** Pool je Replikat gegen `max_connections` rechnen, PgBouncer im
  Transaction-Mode davor, Indizes auf `household_id` in jeder Tabelle,
  Lesereplikate erst wenn gemessen. Migrationen laufen bereits unter einem
  Advisory Lock, also rollout-sicher — aber lange Migrationen blockieren dann
  den Rollout, deshalb Expand/Contract statt Umbauten in einem Schritt.
- **Hintergrundarbeit.** Spätestens hier wird aus der Erinnerungsfrage aus F1.2
  echte Infrastruktur: `event_outbox` bekommt einen Schreiber und einen
  Verarbeiter mit Leader-Election oder Queue.
- **Beobachtbarkeit zuerst.** Ohne den Fix aus 7.7 (`route`-Label) ist jede
  Lastaussage wertlos.

#### F2.4 Etappen

| # | Inhalt | Abnahme |
| --- | --- | --- |
| S0 | Entscheidung + ADR: Beta bleibt lokal, oder Betrieb mit Kosten (F2.5) | ADR in `docs/adr/` |
| S1 | Mandantenmodell: `households`, `household_id` überall, RLS, Isolationstests | Test „fremder Haushalt → 404" je Modul |
| S2 | Registrierung, Verifikation, Passwort-Reset, Rollen wirksam (P2-1) | Integrationstests |
| S3 | Zustandslos: Rate-Limit gemeinsam, Sessions entkoppelt, Konfiguration | 2 Replikate verhalten sich wie 1 |
| S4 | Objektspeicher statt RWO-PVC | Upload/Download unverändert sicher |
| S5 | Argon2-Parameter + Login-Kapazität, erste Lastmessung | Anmeldungen/s dokumentiert |
| S6 | PgBouncer, Indizes, Poolgrößen | Sättigungskurve gemessen |
| S7 | Betrieb: TLS, Ingress, Backups mit Restore-Test, Retention, Alarmzustellung | Threat-Model-Gates erfüllt |
| S8 | Lasttest 500 gleichzeitig als Abnahmekriterium (k6, Open Source, lokal) | Bericht mit p95 je Route |

Reihenfolge ist nicht verhandelbar: **S1 vor allem anderen.** Jede Zeile Code,
die vorher ohne Mandanten geschrieben wird, wird später angefasst.

#### F2.5 Was das kostet — hier hält die 0-Euro-Regel nicht

Ehrlich benannt, ohne Genauigkeit vorzutäuschen. Für 10.000 Konten und 500
gleichzeitige Sitzungen fällt an:

- **Betrieb** (Server/verwaltetes Kubernetes, Datenbank mit Backup, Objektspeicher):
  je nach Anbieter und Selbstbetriebsanteil ein zweistelliger bis niedriger
  dreistelliger Betrag **pro Monat**, nicht einmalig.
- **Domain** jährlich; **TLS** ist über Let's Encrypt kostenlos.
- **Transaktions-E-Mail** für Verifikation und Passwort-Reset. Das ist die
  unauffälligste Position und praktisch nirgends dauerhaft kostenlos in dieser
  Größenordnung. Ohne Mailversand gibt es keine Selbstregistrierung.
- **Backup außer Haus** — ein Backup auf demselben Gerät ist keines.
- **Rechtliches, das kein Softwareproblem ist:** 10.000 fremde Benutzer
  bedeuten DSGVO-Pflichten (Datenschutzerklärung, Impressum, Löschkonzept,
  Auskunftsfähigkeit, Auftragsverarbeitungsverträge mit jedem Dienstleister,
  technisch-organisatorische Maßnahmen). Bei Haustierdaten kommen Angaben über
  den Haushalt und ggf. Tierarztnamen hinzu. Das ist Aufwand und ggf. Beratung.

**Damit ist das eine unternehmerische Entscheidung, keine technische.** Die
beiden konsistenten Wege:

- **A — Beta bleibt, was sie ist:** lokal, zwei bis wenige Benutzer, 0 €. Das
  Pet-Modul (F1) wird gebaut, `household_id` kommt trotzdem von Anfang an rein
  (F2.7), damit der Weg offen bleibt.
- **B — Produkt:** S0–S8, laufende Kosten, Rechtsrahmen, und die Regeln in
  `AGENTS.md` (0 €, kein Remote Access, zwei Benutzer) werden durch neue ADRs
  ersetzt statt stillschweigend gebrochen.

Ein Mittelweg „skalierbar bauen, aber nichts davon betreiben" ist möglich und
kostet nichts außer Disziplin — das ist F2.7.

#### F2.6 Komfortabel und intuitiv — was das konkret heißt

Gilt für das Pet-Modul (F1) genauso wie für den Bestand:

- **Ein Bildschirm je Tier.** Der Dashboard-Endpunkt aus F1/E10 liefert
  nächster Termin, nächste Impfung, aktive Medikamente, offene Artikel und
  Budget in **einer** Abfrage statt sieben. Das ist gleichzeitig die
  skalierungsfreundliche Variante.
- **Vorbelegen statt fragen.** Letzter Tierarzt, letzte Kategorie, heutiges
  Datum, zuletzt genutzte Einheit. Pflichtfelder nur, wo fachlich nötig — steht
  schon in der Pet-Vorlage.
- **Rückgängig statt Rückfrage.** Löschen mit „Rückgängig"-Hinweis ist
  angenehmer als ein Bestätigungsdialog vor jeder Aktion.
- **Mobil zuerst.** Die App wird beim Tierarzt benutzt, auf dem Telefon. Das
  bestehende UI ist responsiv, aber nie unter realen Bedingungen geprüft.
- **Suche über alle Tiere**, sobald es mehr als zwei sind.
- **Bedienbarkeit:** Beschriftungen, Fokusreihenfolge, Kontrast, Fehlermeldungen
  am Feld statt nur als Banner. Heute gibt es eine Statusmeldung oben.
- **Sprache:** die Oberfläche ist deutsch (Abschnitt 4.7). Bei 10.000 Benutzern
  wird Mehrsprachigkeit eine Entscheidung — nachträglich eingezogen ist sie
  teuer, deshalb gehört sie in S0, nicht in S8.

#### F2.7 Sofort sinnvoll, unabhängig von der Entscheidung

Kostet nichts, hält beide Wege offen, verhindert teures Nachrüsten:

1. **`household_id` von Anfang an** in jede neue Pet-Tabelle, auch bei einem
   Haushalt. Der teuerste Fehler wäre, F1 ohne diese Spalte zu bauen.
2. **Nichts Fachliches mehr in den Prozessspeicher.** Der Login-Rate-Limiter
   ist heute schon ein Fehler mit zwei Gesichtern: pro Replikat wirkungslos
   **und** eine Map ohne Eviction. Beides ist klein zu beheben.
3. **`route`-Label reparieren** (7.7), bevor 25 Pet-Routen dazukommen.
4. **`id.IsUUID` überall** (7.9) — 500er verfälschen jede Fehlerrate.
5. **Keine neue Abfrage ohne Index-Gedanken**, keine Liste ohne Obergrenze. Der
   Kalender deckelt bereits bei 366 Tagen; das bleibt das Muster.

### P1 — kleiner Aufwand, sofort spürbar

*Leer.* P1-1 (Ist-Betrag im UI) und P1-5 (`make ci` + Git-Hook) sind am
2026-08-31 erledigt, siehe Protokoll. Der Hook ist **nicht** installiert; das
macht `make install-git-hooks`, wenn Marco es will.

### P2 — echte Lücken im Produkt

| # | Aufgabe | Warum | Scope | Prüfbar durch |
| --- | --- | --- | --- | --- |
| P2-1 | Rollen wirksam machen | `role` wird gespeichert und in den Principal getragen, aber **keine einzige Autorisierungsprüfung** wertet ihn aus — `admin` und `member` können identisch alles. Das Threat Model führt „ownership checks in use cases" als Mitigation gegen Broken Access Control; die gibt es so nicht. Entweder umsetzen oder das Threat Model ehrlich machen. | `backend/internal/auth/`, `httpapi/`, `docs/security/threat-model.md` | Toolchain |
| P2-4 | CRUD-Lücken schließen | Kein Löschen/Bearbeiten für Einkaufsartikel und Ausgaben, kein `GET /expenses/{id}`, keine Paginierung. | `backend/`, `openapi.yaml`, `frontend/` | Toolchain |
| P2-5 | Einkaufslisten erreichbar machen | Beim Bootstrap wird eine geteilte Liste angelegt, danach gibt es keinen Endpoint. Das Frontend hardcodet die UUID `00000000-0000-4000-8000-000000000001`; private Listen sind im Schema vorgesehen, aber unerreichbar. | `backend/internal/shopping/`, `openapi.yaml`, `frontend/src/App.tsx` | Toolchain |
| P2-6 | Benutzerverwaltung (minimal) | `disabled` wird bei Login und Tokenprüfung geprüft, kann aber nirgends gesetzt werden. Kein Passwortwechsel. Bei zwei Benutzern reicht sehr wenig — aber „gar nicht" ist zu wenig. | `backend/internal/auth/`, `httpapi/`, `openapi.yaml` | Toolchain |

### P3 — Betrieb und Härtung

| # | Aufgabe | Warum | Scope | Prüfbar durch |
| --- | --- | --- | --- | --- |
| P3-1 | Audit-Log lesbar machen | Wird geschrieben (nur von `auth`), aber nie gelesen: kein Endpoint, keine Anzeige, keine Retention. Sicherheitsereignisse in einer Tabelle, die niemand ansieht, sind keine Kontrolle. | `backend/`, `openapi.yaml`, Retention-Migration | Toolchain |
| P3-2 | Backup automatisieren | Skripte sind gut, laufen aber nur, wenn jemand sie tippt; keine Verschlüsselung; der geforderte quartalsweise Restore-Test wurde nach Aktenlage nie durchgeführt. Systemd-Timer oder k8s-CronJob, beides kostenlos. | `scripts/`, `docs/architecture/backup.md`, ggf. `kubernetes/` | teils YAML |
| P3-3 | Migrations-Rollback verifizieren | `.down.sql` existiert, wird aber nirgends getestet oder dokumentiert. | `backend/migrations/`, Tests | Toolchain / SQL |
| P3-4 | Git-Remote klären | Branch `main`, kein Remote — die Historie existiert ausschließlich auf dem Laptop. Ein Push ins Nichts fällt nicht auf. Kostenlose Optionen: zweites lokales Bare-Repo auf externem Medium, oder ein Hoster — letzteres ist eine Datenschutzentscheidung für Marco, kein Selbstläufer. | — (Entscheidung) | — |
| P3-5 | `docs/adr/0004-*` | Lücke in der Nummernfolge: bewusst ausgelassen oder verloren? | `docs/adr/` | Markdown |
| P3-6 | Tracing / Logaggregation | `slog` schreibt nach stdout, nichts sammelt es ein; OpenTelemetry kommt im Repo nicht vor. **Erst nach RAM-Entscheidung** (s. 5.1) — kostenlos ist nicht ressourcenfrei. | — | — |
| P3-7 | Doku gegen den Ist-Stand ziehen | Zwei Stellen sind seit 2026-08-31 nachweislich falsch: `docs/security/threat-model.md` und `docs/architecture/overview.md` behaupten, die NetworkPolicies seien wirkungslos (sie sind es nicht, s. 7.2), `docs/architecture/overview.md` sagt außerdem „Calendar remains modeled but is not yet exposed" — seit 2026-08-31 falsch; und das Threat Model kennt weder den Beleg-Download noch die Sichtbarkeitsregel des Kalenders — die Kontrollen dafür (Attachment, Content-Type-Pinning, per-Antwort-CSP, Bindung an die Ausgabe) stehen nur im Code und im Protokoll. | `docs/security/threat-model.md`, `docs/architecture/overview.md` | Markdown |

### Bewusst nicht im Backlog

Entschieden und dokumentiert, nicht „vergessen": Remote Access/TLS/VPN (ADR
`remote-access.md`, Gates im Threat Model), OCR, TOTP/2FA, sowie Alexa und Ring
— beide unter der Kostenregel formal **abgelehnt** (ADR 0005). Home Assistant
und MQTT gelten als vielversprechend, aber nicht installiert; real existiert nur
`integrations/mock`.

`event_outbox` bleibt bewusst liegen: Tabelle und Index existieren, kein
einziger Schreibzugriff im Code, und die in `domain-model.md` benannten Events
existieren nirgends. **Seit dem Kalender (2026-08-31) gibt es aber den ersten
plausiblen Konsumenten:** `reminderMinutes` wird gespeichert und von nichts
zugestellt, und `calendar.event.created` ist in `domain-model.md` bereits
benannt. Wer Erinnerungen wirklich will, braucht beides — einen Schreiber für
`event_outbox` und etwas, das ihn ausliest (Timer, CronJob, Worker). Das ist
eine RAM- und Betriebsentscheidung für Marco, kein Selbstläufer.
`worker/`, `helm/` und `terraform/` sind aus demselben Grund leere Platzhalter.

---

## 7. Beobachtungen (nicht claimen, nur notieren)

> **Seit 2026-09-01 auch als Tickets.** Die Abschnitte 6, 7 und 8 sind nach
> Mission Control übernommen: 2 Epics, 11 Backlog-Aufgaben, 12 Beobachtungen und
> 28 Protokolleinträge, nachzulesen unter <http://127.0.0.1:7777> im Backlog.
> Diese Datei bleibt bis auf Weiteres die verbindliche Quelle — die Ablösung ist
> Ticket MC-0002 und Marcos Entscheidung. Bis dahin gilt: **hier eintragen, dort
> nachschlagen.** Der Import ist wiederholbar und überspringt, was es schon gibt.


Was einem außerhalb des eigenen Scopes auffällt, kommt hierher. Kein Fix ohne
Claim.

### 7.1 `/metrics` ist unauthentifiziert

Registriert am `public`-Mux in `server.go`, also ohne Auth erreichbar. Lokal
und im ClusterIP-Only-Setup vertretbar; die Metriken enthalten keine privaten
Werte. Es ist aber ein Punkt, der **vor jedem Remote Access** entschieden sein
muss — passt zu den Gates in `docs/security/threat-model.md`. Gehört in den
Sprint-5-Abschnitt des Threat Models (P1-3).

### 7.2 Netzwerkisolation: kindnet **erzwingt** NetworkPolicy (korrigiert 2026-08-31)

**Diese Beobachtung stand vorher falsch hier.** Die Annahme „kindnet ignoriert
NetworkPolicy" stammt aus älteren kind-Versionen. Auf diesem Laptop läuft
kind 0.30 mit `kindest/kindnetd:v20250512-df8de77b`, und dieses Image bringt
`kube-network-policies` als NRI-Plugin mit. Nachweis im Cluster:

```
kubectl -n kube-system logs ds/kindnet | grep kube-network-policies
docker exec dev-control-plane nft list tables | grep kindnet
```

Beides ist vorhanden: `Started plugin 10-kube-network-policies` im Log,
`table inet kindnet-network-policies` in nftables.

Empirisch geprüft am 2026-08-31 (Claude Code) mit einem Probe-Pod im Namespace
`default`, gegen die Pod-IPs von API und PostgreSQL:

| Test | Policies aktiv | Policies gelöscht |
| --- | --- | --- |
| `default` → PostgreSQL:5432 (laut Policy verboten) | blockiert | erreichbar |
| `default` → API:8080 (laut Policy verboten) | blockiert | erreichbar |
| `default` → CoreDNS:53 (keine Policy, Kontrolle) | erreichbar | erreichbar |

Gegenprobe für den erlaubten Pfad: API → PostgreSQL funktioniert unter den
Policies weiter. Ein Login mit erfundenem Benutzernamen liefert `401` (nicht
`500`), die DB-Abfrage läuft also. Zusätzlich `rollout restart deploy/homeflow-api`:
der Kaltstart inklusive Migrationen kommt unter den Policies sauber hoch.

Konsequenzen:

- Die Objekte in `kubernetes/base/networkpolicy.yaml` sind **wirksame
  Kontrolle**, nicht bloß Absicht. Sie sind seit 2026-08-31 im Cluster
  angewendet.
- Die RAM-Frage Calico/Cilium (100–150 MiB je Node) ist damit **erledigt** —
  es braucht kein zusätzliches CNI und keine Entscheidung darüber.
- Umgekehrt gilt jetzt: ein falscher Selektor bricht echten Verkehr, statt
  folgenlos zu bleiben. Wer `networkpolicy.yaml` ändert, testet es.
- Offen bleibt der Namespace `homeflow-monitoring`: er ist bewusst nicht
  abgedeckt, und der Prometheus-Scrape auf `/metrics` ist noch nicht real
  geprüft — der Monitoring-Stack ist auf diesem Cluster nie deployt worden
  (keins der drei Images liegt im kind-Node). Die Regel `api-ingress-monitoring`
  erlaubt genau `prometheus` → API:8080; ob der Selektor stimmt, zeigt sich
  erst beim ersten `make monitoring-deploy`.
- `docs/security/threat-model.md` und `docs/architecture/overview.md`
  behaupten laut Protokoll noch die Wirkungslosigkeit. Das ist jetzt falsch und
  muss nachgezogen werden (nicht in meinem Scope, s. Backlog).

### 7.3 Irreführende Variablennamen in `csrf()`

In `server.go` heißen zwei `error`-Rückgaben `hasAccessCookie` /
`hasRefreshCookie` und werden gegen `!= nil` geprüft — die Bedingung ist damit
„beide Cookies **fehlen**". Verhalten korrekt (ohne Cookie-Auth kein
CSRF-Bedarf), Benennung sagt das Gegenteil. Kosmetisch, aber genau die Stelle,
an der später jemand die falsche Korrektur macht.

---

### 7.4 Die Wurzel-`README.md` ist überschrieben

Sie enthält nicht den Projekttext, sondern eine ältere Kopie von
`monitoring/README.md` (19 Zeilen, beginnt mit „# Local monitoring", ohne den
Abschnitt „Alerting rules"). Der deutsche Projekttext mit Sprint-5-Stand,
`curl`-Beispielen und dem Abschnitt „Netzwerkisolation" — im Protokoll für
P1-3 beschrieben — existiert nirgends mehr. Das Repo hat **keinen einzigen
Commit**, aus Git ist nichts zu holen, `backups/` ist leer.

Konsequenzen, die über die Datei hinausgehen:

- Abschnitt 4.1 nennt „die `curl`-Beispiele in `README.md`" als Konsumenten der
  OpenAPI. Dieser Konsument existiert nicht mehr.
- Beobachtung 7.2 verwies auf einen Nachweisbefehl in der README. Auch der ist
  weg; die Methode steht jetzt vollständig in 7.2 selbst.
- Es ist der zweite Beleg für P3-4 (kein Remote, keine Historie): ohne Commits
  ist jedes Überschreiben endgültig.

Marco hat am 2026-08-31 entschieden: nur notieren, nicht neu schreiben.

### 7.5 Cluster-Stand (erledigt 2026-08-31)

Der Cluster lief auf `api:0.4.0`, während das Manifest `0.5.0` verlangte — der
Grund dafür, dass Marco Kalender, Ist-Betrag und Beleg-Download in der App
nicht sah: das ausgelieferte JS-Bundle war das von Sprint 4, null Treffer für
„Kalender" und „Ist-Betrag".

Am 2026-08-31 behoben mit `make docker-build kind-load deploy`. Der Cluster
läuft jetzt auf `0.5.0`, das Bundle enthält alle heutigen UI-Änderungen.
`0.4.0` liegt weiterhin im kind-Node, ein Rückweg ist also offen.

**Merksatz, der bleibt:** Code im Arbeitsverzeichnis ist nicht Code im Cluster.
Zwischen beiden liegen drei Schritte, und `make deploy` allein genügt nicht —
ohne `docker-build` und `kind-load` wird das alte Image weiterverwendet.
Ebenso: ein laufender `kubectl port-forward` **stirbt**, wenn der Pod ersetzt
wird. Nach jedem Deploy neu starten (`make port-forward`).

### 7.6 `frontend/package.json` steht auf 0.4.0

`Makefile` (`VERSION`), `api-deployment.yaml` und `openapi.yaml` stehen auf
0.5.0, `frontend/package.json` auf 0.4.0. Ohne Wirkung — die Zahl wird nirgends
ausgewertet — aber sie taucht in jeder npm-Ausgabe auf und ist damit eine
stille Fehlerquelle beim Lesen von Logs. Vorschlag: `frontend/package.json` in
Abschnitt 4.6 mit aufnehmen. Das ist eine Schnittstellenänderung, deshalb nicht
im Alleingang gemacht.

### 7.7 `route`-Label auf echte Routenmuster korrigiert (erledigt 2026-09-01)

Der am 2026-08-31 gemessene Fehler ist behoben. Die Auth-Middleware übernimmt
nach dem inneren Mux dessen `r.Pattern` in den äußeren Request. Ein API-Test
sichert das Muster ab; am laufenden Prometheus wurden am 2026-09-01 getrennt
`GET /api/v1/shopping/items`, `POST /api/v1/auth/browser/login`, `GET /health`
und `GET /metrics` nachgewiesen.

Ursache: die Metrik-Middleware liegt außen und liest `r.Pattern` an ihrer
eigenen Request-Instanz. Getroffen hat dort nur der äußere Mux mit dem Muster
`/api/`; das genauere Muster des inneren `private`-Mux steht in einer neuen
Request-Instanz, die die Middleware nie sieht.

Die Kardinalität bleibt begrenzt: Es werden ausschließlich registrierte
Routenmuster erfasst, niemals Pfad-IDs oder Benutzereingaben. Grafana und der
Alarm `HomeflowApiLatency` können damit tatsächlich nach Route unterscheiden.

### 7.8 `internal/domain/` ist toter Rest

`internal/domain/users/model.go` wird von niemandem importiert; die Rolle wird
in `auth` separat gehalten. `internal/domain/expenses/` und
`internal/domain/receipts/` sind leere Verzeichnisse. `internal/domain/calendar`
ist am 2026-08-31 weggefallen, weil das Kalendermodul wie die drei anderen
unter `internal/calendar/` liegt. Der Rest ist Aufräumarbeit, kein Fehler.

### 7.9 Kaputte UUIDs sind nur in zwei von vier Modulen abgefangen

`id.IsUUID` existiert seit 2026-08-31 und wird von `receipts` und `calendar`
benutzt; `shopping` und `expenses` reichen den Pfad- bzw. Query-Wert weiter an
eine `uuid`-Spalte, und der Datenbankfehler landet als 500 beim Aufrufer. Am
laufenden System gemessen:

| Anfrage | heute |
| --- | --- |
| `POST /api/v1/shopping/items/nonsense/complete` | **500** |
| `GET /api/v1/shopping/items?listId=nonsense` | **500** |
| `POST /api/v1/expenses/nonsense/paid` | **500** |
| `GET /api/v1/expenses/nonsense/receipts` | 400 |
| `GET /api/v1/calendar/events/nonsense` | 400 |

Kein Datenleck — die Fehlermeldung nach außen ist die generische Problem-Antwort
—, aber ein 500 ist eine Falschaussage über die Ursache, und die drei Zeilen
oben erzeugen 5xx-Rauschen, das direkt in `HomeflowApiErrorRatio` einläuft.
Der Fix ist jeweils eine Zeile mit `id.IsUUID`. Nicht gemacht, weil außerhalb
des Scopes von P2-2 und P2-3; passt am ehesten zu P2-4.

### 7.10 Der Login-Rate-Limiter räumt nur nach erfolgreichen Anmeldungen auf

`loginRateLimiter.attempts` ist eine `map[string]attemptWindow` im
Prozessspeicher, Schlüssel ist `IP|Benutzername`. `Reset(key)` löscht einen
Eintrag — aufgerufen wird es aber **nur nach einer erfolgreichen Anmeldung**
(`server.go`, beide Login-Pfade). Ein Schlüssel, unter dem nur fehlgeschlagene
Versuche liefen, bleibt für immer stehen: das Zeitfenster wird beim nächsten
Treffer desselben Schlüssels lazy zurückgesetzt, der Eintrag selbst nie
entfernt. Es gibt keinen Aufräum-Goroutine.

Zwei Wirkungen, beide heute folgenlos, beide relevant, sobald mehr als dieser
Laptop im Spiel ist:

- **Unbegrenztes Wachstum.** Jede neue Kombination aus Quell-IP und
  Benutzername legt einen Eintrag an, der nie verschwindet. Wer von außen
  Benutzernamen durchprobiert, lässt den Speicher wachsen.
- **Wirkungslos bei mehreren Replikaten.** Der Zustand liegt im Prozess; mit
  zwei Pods hat ein Angreifer die doppelte Anzahl Versuche.

Lokal mit zwei Benutzern ist beides ohne Bedeutung — deshalb Beobachtung und
kein Fix. Vor jedem Remote Access und vor F2/S3 gehört es behoben: Einträge
beim Ablauf des Fensters entfernen und den Zustand aus dem Prozess holen.

### 7.11 `homeflow_expenses_due` fragt die Datenbank bei jedem Scrape

Die Gauge ist ein `GaugeFunc`, der Callback läuft also bei **jedem** Einsammeln
der Metriken — bei `scrape_interval: 15s` viermal pro Minute ein
`SELECT count(*) FROM expenses WHERE status IN ('planned','open') AND due_date
<= CURRENT_DATE`. Auf `expenses` gibt es dafür den passenden Index
(`expenses_due_idx`), und bei dieser Datenmenge kostet es nichts.

Zwei Punkte trotzdem für den Monitoring-Claim:

- Ein Fehler im Callback liefert stillschweigend `0`, nicht „kein Wert". Der
  Alarm `HomeflowExpensesOverdue` wird dadurch im Fehlerfall nicht falsch
  auslösen, aber er wird auch nicht auffallen.
- Die Abfrage teilt sich den Pool von fünf Verbindungen mit den Anfragen der
  Benutzer. Solange nur `/metrics` gescrapet wird, ist das nichts; unter F2
  (mehr Daten, mehr Mandanten, mehr Replikate — jedes mit eigenem `/metrics`)
  wird daraus eine Dauerlast, die mit der Anzahl der Pods multipliziert.

Liegt in Codex' Scope (Abschnitt 4.2), deshalb hier und nicht als Fix.

### 7.12 Mission Control läuft parallel — Austausch.md bleibt maßgeblich

Seit 2026-09-01 läuft ein lokales Cockpit unter <http://127.0.0.1:7777>
(`missioncontrol/`, systemd-Benutzerdienst, ~12 MiB), seit 0.2.0 mit
Menüeintrag als eigene Anwendung. Es zeigt die Pod-Abhängigkeiten, das Log des
ausgewählten Pods und ein dateibasiertes Ticketsystem mit den Spalten
Backlog → Ready → In Bearbeitung → Test → Fertig. **Mission Control versioniert
sich selbst und folgt nicht HomeFlows `VERSION`.**

**Für die Zusammenarbeit ändert sich vorerst nichts:** diese Datei bleibt die
verbindliche Quelle, Claims und Protokoll laufen unverändert hier. Tickets sind
bis zur Umstellung ein zusätzlicher Kanal, kein Ersatz. Die Umstellung ist in
Ticket MC-0002 vorbereitet und wird von Marco ausgesprochen, nicht von uns.

Wer ein Ticket lesen oder beantworten will, braucht das Cockpit nicht:
`missioncontrol/tickets/MC-*.md` sind gewöhnliche Markdown-Dateien, und ein von
Hand angehängter Abschnitt `## Kommentar — <Name> · <RFC3339>` wird beim
nächsten Lesen als Kommentar erkannt.

## 8. Protokoll

> **Seit 2026-09-01 auch als Tickets.** Die Abschnitte 6, 7 und 8 sind nach
> Mission Control übernommen: 2 Epics, 11 Backlog-Aufgaben, 12 Beobachtungen und
> 28 Protokolleinträge, nachzulesen unter <http://127.0.0.1:7777> im Backlog.
> Diese Datei bleibt bis auf Weiteres die verbindliche Quelle — die Ablösung ist
> Ticket MC-0002 und Marcos Entscheidung. Bis dahin gilt: **hier eintragen, dort
> nachschlagen.** Der Import ist wiederholbar und überspringt, was es schon gibt.


Abgeschlossene Claims, neueste zuerst. Ein bis zwei Sätze — was geändert wurde
und was der andere davon wissen muss.

| Datum | Owner | Ergebnis |
| --- | --- | --- |
| 2026-09-01 | Claude Code | **Tickets mit dem Repository verbunden (Mission Control 0.5.0).** Zwei Hälften, weil das Repo zwei Hälften Historie hat. **Dateien, trägt heute:** jedes Ticket kann einen `scope` aus Pfaden und Mustern führen (`backend/internal/pets/`, `backend/**/*_test.go`, `Makefile`). Daraus entsteht (a) die Zuordnung jeder unversionierten Änderung zu dem Ticket, dessen Scope sie trifft — was zu keinem passt, steht sichtbar unter „Änderungen ohne Ticket" — und (b) die **Kollisionswarnung**: zwei Tickets *in Bearbeitung* auf überlappenden Pfaden werden gemeldet. Damit ist die offene Frage aus MC-0002 beantwortet: **ein Ticket in „In Bearbeitung" mit Scope ist der Claim**, es braucht kein zweites Konzept. **Commits, trägt ab dem ersten:** ein Commit, dessen Betreff oder Rumpf einen Ticketschlüssel enthält (`git commit -m \"HF-0042 …\"`), wird dem Ticket zugeordnet. Mission Control ruft nur lesende git-Kommandos auf und schreibt nichts. Verifiziert: neues Paket `internal/repo` mit 87 % Abdeckung (Mustervergleich in allen drei Schreibweisen, Kollisionserkennung gegen den realen Fall „Codex hält backend/internal, Makefile frei", git-Log-Parsen inkl. Schlüssel aus Betreff und Rumpf, Übersetzung der Statuscodes); am echten Repo gegengeprüft: 257 unversionierte Pfade, davon 10 einem Ticket zugeordnet; Kollisionsfall einmal echt hergestellt (MC-0001 ↔ MC-0003 über `Makefile`) und wieder aufgelöst. **Zwei Mängel dabei behoben:** `git status` fasst ungetrackte Verzeichnisse zu einer Zeile zusammen — ohne `-uall` hätte ein Scope wie `missioncontrol/internal/logs/` nie getroffen; und leere Listen kamen als `null` statt `[]` an. **Der Zustand, den die Ansicht zeigt, ist der eigentliche Befund:** dieses Repo hat **keinen einzigen Commit** und kein Remote. Die Commit-Hälfte der Verbindung ist damit gebaut, aber leer, und HF-0010 (P3-4) ist keine Formalie mehr — fehlende Historie hat in dieser Session zweimal Arbeit gekostet. |
| 2026-09-01 | Claude Code | **Log-Ansicht auf Kibana-Discover-Niveau gebracht (Mission Control 0.4.0).** Der Kern ist ein Parser für die **fünf Formate, die in diesem Cluster wirklich vorkommen** — slog-JSON (HomeFlow-API), logfmt (Prometheus und Grafana, unterschiedliche Feldreihenfolge), klog (kube-state-metrics) und PostgreSQLs eigenes Format; alles andere bleibt als Text sichtbar, das Level wird notfalls aus Wörtern wie `panic` geraten. Die Levelnamen werden vereinheitlicht (PostgreSQLs `FATAL`, Grafanas `info`, klogs `E`-Präfix → ERROR/WARN/INFO/DEBUG), damit **ein** Filter über alle fünf Formate wirkt. Darauf aufbauend: Level-Chips mit Zählern, nach Level gestapeltes Histogramm mit Fehlern in Rot, Feld-Seitenleiste mit den häufigsten Werten je Feld (ein Klick = Filter), aufklappbare Zeilen mit Feldtabelle und Rohzeile, Volltextsuche. **Wichtigste Design-Entscheidung:** die Zähler werden über *alle* Zeilen berechnet, nicht über die gefilterten — ein Filter, der die Fehler versteckt, darf nicht auch verstecken, dass es Fehler gibt. Ausgewertet wird serverseitig, weil Histogramm und Zähler zueinander passen müssen. Verifiziert: neues Paket `internal/logs` mit 87 % Abdeckung, getestet gegen **echte, aus diesem Cluster kopierte Zeilen** aller fünf Formate; im Browser gegengeprüft an Prometheus (200 WARN, Felder `scrape_pool`, `component`, `target`) und an PostgreSQL (ERROR 2 · WARN 2 · INFO 89, Formate `postgres 46 · text 47`) — der ERROR-Filter zeigt zwei echte Constraint-Verletzungen, die Zähler bleiben dabei vollständig. `make mission-control-ci` grün, alle vier Pakete zwischen 83 und 87 % Abdeckung. **Panne unterwegs, weil sie sich wiederholen kann:** beim Ersetzen des Log-Blocks in `web/app.js` habe ich per Textbereich auch den Ticket- und Versionsteil mit erwischt; ohne Commits im Repo war nichts wiederherzustellen, ich habe ihn neu geschrieben. Das Repo hat weiterhin **keinen einzigen Commit** (P3-4 / HF-0010) — das ist der zweite Vorfall dieser Art nach der überschriebenen README. |
| 2026-09-01 | Claude Code | **Historie nach Mission Control übernommen, Board auf Jira-Vokabular umgebaut (MC 0.3.0).** Das Ticketmodell kennt jetzt **Projekt** (`HF`/`MC`, im Schlüssel: `HF-0042`), **Typ** (Epic, Task, Bug, Beobachtung, Protokoll), **Priorität**, **Epic-Zuordnung**, **Komponente** und **Version je Projekt** — Mission Control versioniert weiter unabhängig von HomeFlow. **Import:** `cmd/mcimport` hat aus dieser Datei **53 Tickets** erzeugt — 2 Epics (F1, F2), 11 Backlog-Aufgaben, 12 Beobachtungen, 28 Protokolleinträge mit ihrem ursprünglichen Datum und Status `Fertig`. Wortlaut und Daten bleiben unverändert; der Import ist über den Titel idempotent, ein zweiter Lauf legt nichts doppelt an. **Drei Ansichten statt einer Liste:** Board (fünf Spalten, Protokoll standardmäßig ausgeblendet, sonst mauern 28 fertige Einträge die Spalte „Fertig" zu), Backlog (nach Epics gruppiert, dann nach Typ) und **Versionen**. Die Versionsseite liest die Zahlen dort, wo sie stehen — `Makefile`, `openapi.yaml`, `package.json`, laufendes Image im Cluster, Images aller Pods, `go.mod` beider Module, `kubectl version` — und markiert Abweichungen rot. Sie zeigt sofort den bekannten Fall: Repository 0.15.0, Cluster 0.5.0. Verifiziert: `make mission-control-ci` grün mit 85 % Abdeckung in allen drei Paketen, Importer gegen die **echte** `Austausch.md` getestet (Vollständigkeit je Typ, Datumserhalt, Nummern der Beobachtungen, Idempotenz, Round-Trip), Board/Backlog/Versionen im Browser durchgesehen. `make ci` als Ganzes weiterhin rot an Codex' zwei unformatierten Dateien (MC-0005). **Diese Datei bleibt verbindlich** — die Abschnitte 6, 7 und 8 tragen jetzt einen Hinweis auf ihre Ticket-Entsprechung, die Ablösung selbst bleibt MC-0002 und Marcos Entscheidung. |
| 2026-09-01 | Claude Code | **Mission-Control-Backlog abgearbeitet, soweit er mir gehört.** MC-0003 erledigt: die Wurzel-`Makefile` hat jetzt `mission-control`, `-ci`, `-install`, `-status`, `-logs`, `-stop`, und `mission-control-ci` läuft in `make ci` mit — die Ziele rufen nur `$(MAKE) -C missioncontrol`, damit es keine zwei Wahrheiten gibt. Codex' Claim umfasste die Makefile nicht, geprüft vor dem Anfangen. **Das Ticket steht auf `Test`, nicht auf `Fertig`:** `make ci` als Ganzes ist nicht grün, weil `fmt-check` an `backend/internal/pets/{measurement_service,service_test}.go` abbricht — beide in Codex' aktivem Claim, deshalb nicht angefasst (ein Formatierungslauf ist ohnehin ein eigener Claim). Alle anderen Stufen sind grün, ebenso `make mission-control-ci`. Daraus **MC-0005** für Codex. **MC-0001 (Cluster-Deploy) bewusst nicht ausgeführt:** der Rückstand ist inzwischen 0.5.0 im Cluster gegen 0.15.0 im Repo, aber ein Image aus diesem Arbeitsstand würde Codex' halbfertigen Audit mitnehmen. Wartet auf seinen Claim-Abschluss. **MC-0002** um einen vollständigen Migrationsvorschlag ergänzt: was Dokument bleibt (Abschnitte 1, 2, 4, 5 → `AGENTS.md` bzw. ein neues `docs/architecture/contracts.md`), was Ticket wird (3, 6, 7, 8), der einzige echte Neubau (Datei-Scope am Ticket, damit „In Bearbeitung" = Claim), sechs Migrationsschritte und drei ehrliche Gegenargumente. Empfehlung dort: Schritte 1 und 2 sofort, der Rest erst nach ein paar Wochen echter Nutzung. **MC-0004** (eigener kind-Cluster) bleibt Marcos Entscheidung, Preis steht mit 1,03 GiB im Ticket. |
| 2026-09-01 | Claude Code | **Mission Control 0.2.0: Linux-Anwendung, Log-Ansicht, Board-Spalten, eigene Versionierung.** (1) **Anwendung:** `make install` legt jetzt zusätzlich Menüeintrag und Icon an; `desktop/launch.sh` startet den Dienst falls nötig, wartet auf Bereitschaft und öffnet ein eigenes Fenster. Mit Chromium/Chrome/Brave/Edge im App-Modus (ohne Adresszeile und Tabs), **mit Firefox nicht** — dort wurde der App-Modus entfernt, es gibt ein normales eigenes Fenster plus die Menü-Aktion „Ohne Browserleisten öffnen" (Vollbild). Auf diesem Laptop ist nur Firefox installiert. Ein natives GTK-Fenster hätte `cgo` und `webkit2gtk` verlangt — bewusst nicht. (2) **Log unter der Cluster-Übersicht:** `GET /api/logs` ruft `kubectl logs`, folgt der Auswahl im Graphen, 50–2000 Zeilen, optional mitlaufend, Warnungen und Fehler eingefärbt. Pod-, Container- und Namespace-Namen werden gegen das Kubernetes-Namensschema geprüft, bevor sie kubectl erreichen, und der Namespace muss einer der beobachteten sein — sonst wäre über die Adresszeile jedes Log im Cluster abziehbar; dafür gibt es zehn Negativtests (Flags im Podnamen, Leerzeichen, Pfade, fremder Namespace). (3) **Board:** Backlog → Ready → In Bearbeitung → Test → Fertig, Spaltenreihenfolge kommt aus `GET /api/board`, damit Oberfläche und Speicher nicht auseinanderlaufen; die alten Statusnamen werden beim Lesen abgebildet statt still zurückgesetzt. (4) **Versionierung:** jedes Ticket trägt eine Mission-Control-Version, Vorgabe ist die laufende (0.2.0); Tickets ohne Feld übernehmen sie beim nächsten Schreiben. **Mission Control folgt ausdrücklich nicht HomeFlows `VERSION`** — eigenes Produkt, eigener Lebenszyklus. (5) **Cluster-Unabhängigkeit:** neue Flags `-kubeconfig` und `-context`; das Cockpit muss nicht in dem Cluster laufen, den es beobachtet. Verifiziert: `make ci` grün (85 % in allen drei Paketen), Board und Ticketwechsel im Browser durchgespielt, Log am echten API-Pod gelesen, Desktop-Eintrag mit `desktop-file-validate` geprüft, Dienst nach Neustart aktiv (12 MiB RSS). **Offen und Marcos Entscheidung:** ein *eigener* kind-Cluster für Mission Control — der bestehende Node belegt gemessen **1,03 GiB** von 7,1 GiB; ein zweiter kostet ungefähr dasselbe. Siehe Ticket MC-0004. |
| 2026-09-01 | Codex | **Pet E10 Dashboard und damit F1 abgeschlossen (Version 0.15.0).** `GET /api/v1/pets/{petId}/dashboard?month=YYYY-MM` liefert in genau einem haushaltsgeprüften SQL-Roundtrip Stammdatenbezug, Akten-/Dokument-/Medikament-/Einkaufszähler, letztes Gewicht, nächste sichtbare Termin-/Impffälligkeit sowie Budget-, Plan- und Ist-Summen des Monats. Der Endpunkt materialisiert keinen Zustand; Detailverläufe bleiben lazy. Die deutsche UI lädt die Übersicht erst beim Aufklappen. Verifiziert: `make lint`, `make test`, `make frontend-test` (46 Tests), OpenAPI-Routenabgleich, YAML-Parse, `kubectl kustomize` und `make integration-test` aus leerem Wegwerf-PostgreSQL inklusive aggregierter Werte und Fremdhaushalt=404; Container entfernt. Kein Deployment/Browser-Klicktest; 0 €, kein neuer Prozess und kein zusätzliches RAM-Budget. |
| 2026-09-01 | Codex | **Pet E9 Messwerte und abgeleitete Erinnerungen abgeschlossen (Version 0.14.0).** `weight_entries` und `lab_results` sind per zusammengesetztem FK haushalts-/tiergebunden, indiziert und auf 100 Treffer begrenzt; Dezimalwerte bleiben als API-Strings exakt und werden als PostgreSQL-`numeric` gespeichert. HomeFlow bewertet keine Laborreferenzen. `GET /api/v1/reminders` leitet beim Lesen manuelle Impffälligkeiten und geplante sichtbare Tiertermine ab (max. 366 Tage/100 Treffer), ohne Scheduler, Queue, Push oder Mail. Deutsche UI ergänzt. Verifiziert: `make lint`, `make test`, `make frontend-test` (46 Tests), OpenAPI-Routenabgleich, YAML-Parse, `kubectl kustomize` und `make integration-test` aus leerem Wegwerf-PostgreSQL; exakte Dezimal-Roundtrips und Fremdhaushalt für Gewichte/Labor/Erinnerungen geprüft, Container entfernt. Kein Deployment/Browser-Klicktest; 0 €, kein neuer Prozess und kein zusätzliches RAM-Budget. |
| 2026-09-01 | Codex | **Pet E8 medizinische Dokumente abgeschlossen (Version 0.13.0).** Die eigene haushalts- und tiergebundene Tabelle `documents` trennt medizinische Unterlagen vollständig von finanziellen `receipts`; Upload, Liste und Download prüfen die Haushaltsmitgliedschaft. Die Byteablage erbt ohne neue Komponente den gehärteten lokalen `LocalStorage`: 10 MiB, Magic Bytes für JPEG/PNG/PDF, SHA-256, zufälliger Schlüssel, `0600`, Traversal-Schutz; Downloads bleiben `attachment` mit gepinntem MIME-Type und Sandbox-CSP. Neu: drei Pet-Dokumentrouten, kontrollierte Dokumenttypen und deutsche UI. Verifiziert: `make lint`, `make test`, `make frontend-test` (46 Tests), OpenAPI-Routenabgleich, YAML-Parse, `kubectl kustomize` sowie `make integration-test` aus leerem Wegwerf-PostgreSQL inklusive Cross-Household-Liste/Download; Container entfernt. Kein Deployment/Browser-Klicktest/OCR/Parser; 0 €, kein zusätzlicher Prozess/RAM, aber Belege und Dokumente teilen weiterhin das knappe 1-GiB-Datei-PVC. |
| 2026-09-01 | Claude Code | **Mission Control v0.1.0 gebaut — Cockpit und Ticketsystem, läuft als Autostart.** Neues, eigenständiges Go-Modul `missioncontrol/` ohne externe Abhängigkeiten (nur Standardbibliothek, kein npm, kein Build-Schritt für die Oberfläche). **Cluster-Ansicht:** je Namespace **ein** `kubectl get pods,services,networkpolicies -o json`, daraus ein Abhängigkeitsgraph — Service→Pod über Selektoren, Pod→PVC, Pod→ConfigMap/Secret (auch über `envFrom` und `env.valueFrom`), Pod→Workload über `ownerReferences`, und die NetworkPolicy-Regeln als „darf erreichen"/„akzeptiert von" inklusive sichtbarer default-deny-Abriegelung. Jede Kante nennt ihr Quellobjekt; ein Service ohne Backend und ein Policy-Selektor ohne Treffer werden als auffällig markiert. Klick auf einen Kasten isoliert ihn und schreibt seine Abhängigkeiten in Sätzen aus. **Ticketsystem:** eine Datei je Ticket in `missioncontrol/tickets/`, Markdown mit kleinem Kopf, atomar geschrieben — beide Assistenten können ohne laufendes Cockpit lesen und schreiben, unbekannte Kopfzeilen werden ignoriert, von Hand angehängte Kommentare werden gelesen. **Autostart:** systemd-**Benutzer**dienst (kein root), `missioncontrol/install.sh` bzw. `make install`; Abschalten mit `make stop`. Verifiziert: `make ci` grün (Abdeckung 84/81/85 % in cluster/httpd/tickets, Graph-Tests gegen aufgezeichnete Objekte des echten Clusters), Ende-zu-Ende im Browser (Ticket angelegt, kommentiert, Status gewechselt, Datei auf der Platte geprüft), Dienst überlebt Neustart mit erhaltenen Tickets, Loopback-Bindung nachgemessen und die Weigerung bei fremder Adresse ausgelöst. **Gemessener Bedarf: rund 12 MiB RSS, cgroup-Spitze 24 MiB inklusive kubectl; Unit deckelt bei 128 MiB.** Kosten: keine. **Zwei Fallen unterwegs, die dokumentiert sind:** ein systemd-*Benutzer*dienst stirbt mit `218/CAPABILITIES`, wenn man ihm `ProtectKernelTunables`/`ProtectKernelModules`/`ProtectControlGroups` mitgibt — die Unit enthält jetzt nur, was der Benutzer-Manager wirklich kann; und drei `kubectl`-Aufrufe je Namespace kosteten 6 Prozesse à ~45 MiB pro Zyklus, deshalb der eine kombinierte Aufruf. **Austausch.md bleibt unverändert die verbindliche Quelle** — die Ablösung ist Ticket MC-0002 und Marcos Entscheidung. Die Wurzel-`Makefile` habe ich **nicht** angefasst, weil sie in Codex' E7-Claim liegt; die `mission-control-*`-Ziele wandern nach Ticket MC-0003 nach oben. |
| 2026-09-01 | Codex | **Pet E7 Tierbudgets und Ausgaben abgeschlossen (Version 0.12.0).** Migration `000009` bindet alle Ausgaben verpflichtend an den Haushalt, erlaubt einen per zusammengesetztem FK auf denselben Haushalt begrenzten optionalen Tierbezug und führt eindeutige Integer-Cent-Budgets je Tier/Monat/Kategorie ein. Bestehende Ausgaben-, Zusammenfassungs- sowie Beleglisten/-downloads prüfen nun die Haushaltsmitgliedschaft. Neu: auf 100 begrenzte Ausgabenlisten mit Tierfilter, `GET/PUT /api/v1/pets/{petId}/budgets` und `GET /api/v1/pets/{petId}/expense-summary?period=YYYY[-MM]`; deutsche UI für Tierzuordnung, Budgetpflege und Kategorienauswertung. Verifiziert: `make lint`, `make test`, `make frontend-test` (45 Tests), OpenAPI-Routenabgleich, YAML-Parse, `kubectl kustomize` sowie `make integration-test` aus leerem Wegwerf-PostgreSQL; Fremdhaushalt sieht weder Tierausgaben noch Budgets, Container entfernt. Kein Deployment/Browser-Klicktest; 0 €, keine neue Dauerkomponente und kein zusätzliches RAM-Budget. |
| 2026-09-01 | Codex | **Pet E6 Einkaufslisten und P2-5 abgeschlossen (Version 0.11.0).** `shopping_lists` und `shopping_items` tragen nun verpflichtend `household_id`; alle Create/List/Complete-Abfragen prüfen Mitgliedschaft und Sichtbarkeit. `shopping_item_pets` bildet Artikel↔Tiere als n:m mit zwei zusammengesetzten Haushalts-FKs ab (max. 20 eindeutige Tiere je Artikel). Neu: `GET /api/v1/shopping/lists`, Tierfilter und harte 100er-Grenze; das Frontend lädt die Listen dynamisch statt der falschen hart codierten UUID und erlaubt mehrere Tierzuordnungen. Verifiziert: `make lint`, `make test`, `make frontend-test` (45 Tests), OpenAPI-YAML, `kubectl kustomize` und `make integration-test` gegen anschließend entfernten Wegwerf-PostgreSQL; Listen-ID, n:m-Roundtrip, Tierfilter und Fremdhaushalt ohne Datenleck geprüft. Kein Deployment/Browser-Klicktest; 0 €, keine neue Komponente. |
| 2026-09-01 | Codex | **Pet E5 Medikamente und Bestand abgeschlossen (Version 0.10.0).** Haushaltsspezifische `medications` und per zusammengesetzten FKs an denselben Haushalt gebundene `pet_medications` werden atomar in einer Transaktion angelegt. Dokumentiert werden Medikament/Wirkstoff/Form, frei eingegebene Dosis und Häufigkeit, Zeitraum sowie exakter nichtnegativer Dezimalbestand (String-API, max. drei Nachkommastellen); HomeFlow berechnet weder Dosierung noch Verbrauch. `GET/POST /api/v1/pets/{petId}/medications`, auf 100 begrenzte Liste, OpenAPI und deutsche UI ergänzt. Verifiziert: `make lint`, `make test`, `make frontend-test` (45 Tests), OpenAPI-YAML, `kubectl kustomize` und `make integration-test` gegen anschließend entfernten Wegwerf-PostgreSQL inklusive exaktem `12.500`-Roundtrip und Fremdhaushalt=404. Kein Deployment/Browser-Klicktest; 0 €, keine neue Dauerkomponente. |
| 2026-09-01 | Codex | **Pet E4 Tiertermine und mandantenfähiger Kalender abgeschlossen (Version 0.9.0).** `calendar_events` hat nun eine verpflichtende `household_id` sowie optionale, per zusammengesetztem FK auf denselben Haushalt begrenzte `pet_id` und kontrollierte `appointment_type`; bestehende Termine wurden dem lokalen Standardhaushalt zugeordnet. Sämtliche PostgreSQL-CRUD-Abfragen prüfen jetzt zusätzlich die Haushaltsmitgliedschaft, private Sichtbarkeit gilt weiterhin innerhalb dieses Haushalts. `GET /calendar/events` unterstützt den indizierten Filter `petId`; das bestehende Kalenderformular kann ein Tier und Tierarzt/Impfung/Medikament/Pflege/Sonstiges zuordnen. Verifiziert: `make lint`, `make test`, `make frontend-test` (45 Tests), OpenAPI-YAML, Routen-/Versionsabgleich, `kubectl kustomize` und `make integration-test` gegen anschließend entfernten Wegwerf-PostgreSQL; Fremdhaushalt sieht selbst einen geteilten Tiertermin nicht. Kein Deployment/Browser-Klicktest; 0 €, keine neue Komponente. |
| 2026-09-01 | Codex | **Pet E3 Impfungen abgeschlossen (Version 0.8.0).** `vaccinations` ist verpflichtend haushalts- und tiergebunden, besitzt Historien- und Fälligkeitsindizes und speichert Impfstoff, Impfdatum, optional Praxis/Charge/Notiz sowie ein ausschließlich manuell eingegebenes `next_due_date`; Schema und Service verhindern Zukunftsimpfungen und eine Fälligkeit vor dem Impfdatum. `GET/POST /api/v1/pets/{petId}/vaccinations`, absteigende Keyset-Paginierung (30/100), OpenAPI und deutsche UI ergänzt; keine Impfentscheidung oder Intervallberechnung. Verifiziert: `make lint`, `make test`, `make frontend-test` (45 Tests), OpenAPI-YAML, `kubectl kustomize kubernetes/base` und `make integration-test` gegen anschließend entfernten Wegwerf-PostgreSQL inklusive Fremdhaushalt und Cursor-Folgeseite. Kein Deployment/Browser-Klicktest; 0 €, keine neue Dauerkomponente. |
| 2026-09-01 | Codex | **Pet E2 Patientenakte abgeschlossen (Version 0.7.0).** Neue haushaltsgebundene Tabelle `medical_records` mit kontrollierten Typen, chronologischer Timeline, begrenzten Text-/Detailfeldern und zusammengesetztem FK `(household_id, pet_id)`; damit kann ein Datensatz nicht an ein Tier eines anderen Haushalts gebunden werden. `GET/POST /api/v1/pets/{petId}/medical-records`, absteigende Keyset-Paginierung (30/100), OpenAPI und deutsche UI direkt unter jedem Tier ergänzt. UI und API stellen ausdrücklich nur Benutzerdokumentation dar und erzeugen keine Diagnosen, Dosierungen oder Behandlungsempfehlungen. Verifiziert: `make lint`, `make test`, `make frontend-test` (45 Tests), OpenAPI-YAML, `kubectl kustomize kubernetes/base` sowie `make integration-test` gegen einen anschließend entfernten Wegwerf-PostgreSQL; Cross-Household-Lesen und Cursor-Folgeseite laufen gegen echtes PostgreSQL. Kein Deployment und kein Browser-Klicktest. Kosten 0 €, keine neue Dauerkomponente, kein zusätzliches RAM-Budget. |
| 2026-09-01 | Codex | **Pet E1 und skalierbare Pet-Grundlage abgeschlossen (Version 0.6.0).** Migration `000003` führt `households`, `household_members` und zwingend haushaltsgebundene `pets` ein; die vorhandenen genau zwei Konten bleiben erhalten und werden dem lokalen Standardhaushalt zugeordnet. Pet-CRUD (`GET/POST /api/v1/pets`, `GET/PATCH /api/v1/pets/{id}`), reversible Archivierung, deutsche UI, OpenAPI und Doku ergänzt. Jede Pet-SQL-Abfrage leitet den Haushalt aus der Mitgliedschaft des angemeldeten Benutzers ab; fremder Haushalt ergibt 404. Listen sind auf 50/100 Einträge begrenzt, verwenden einen UUID-Keyset-Cursor und besitzen einen passenden Haushalts-/Aktiv-/ID-Index. **Bewusste Grenze:** Das ist mandantenfähige Pet-Grundlage, noch kein vollständiges S1 — Shopping, Calendar, Expenses und Receipts sind noch nicht vollständig mit `household_id` nachgerüstet, PostgreSQL-RLS ist deshalb noch nicht aktiviert. Zwei-Benutzer-Limit, lokaler Betrieb, kein Remote Access und 0-€-Regel bleiben unverändert. Verifiziert: `make lint`, `make test`, `make frontend-test`, OpenAPI-YAML-Parse, Routenabgleich, `kubectl kustomize kubernetes/base`; `make integration-test` gegen Wegwerf-PostgreSQL einschließlich Cross-Household-Test grün, Container anschließend entfernt. Kein Browser-Klicktest und kein Deployment durchgeführt. |
| 2026-09-01 | Codex | **Sprint 5 Monitoring abgeschlossen.** Prometheus 3.13, Grafana OSS 13.2 und kube-state-metrics 2.18 lokal im Namespace `homeflow-monitoring` ausgerollt; ausschließlich ClusterIP, zwei neue 1-GiB-PVCs, zufälliges Grafana-Passwort nur im Secret, sieben Tage/750 MiB Retention, kein Alertmanager und keine externe Telemetrie. kube-state-metrics beobachtet per Least Privilege nur Pods, Deployments, StatefulSets, ReplicaSets, PVCs und Nodes; keine Secrets/Webhooks/Zertifikate. `promtool check config` bestätigt ein Rule-File und alle zehn Regeln, alle vier Ziele (`homeflow-api`, `kube-state-metrics`, `kubernetes-cadvisor`, `prometheus`) sind `up`, Grafana-Datenquelle `OK`, Dashboard geladen. Den Fehler aus 7.7 behoben und mit Unit-/laufendem Prometheus-Test verifiziert: authentifizierte API-Routen haben wieder einzelne stabile `route`-Labels. `make ci` grün; keine Integrationstests nötig, da keine Migration/Repository-SQL geändert wurde. Kosten 0 €, keine öffentliche Freigabe, keine bestehenden Volumes gelöscht. |
| 2026-08-31 | Claude Code | **Cluster auf 0.5.0 gebracht.** Marco sah den Kalender in der App nicht; Ursache war nicht der Code, sondern der Deployment-Stand: der Pod lief auf `api:0.4.0`, und das ausgelieferte Bundle hatte null Treffer für „Kalender" und „Ist-Betrag". `make docker-build kind-load deploy` auf seine Zustimmung hin ausgeführt, Rollout sauber durch. Verifiziert am ausgelieferten Bundle: „Kalender", „Ist-Betrag", „Belege", „Ganztägig" und „Nur für mich" jeweils vorhanden; `/health` 200. Beobachtung 7.5 damit erledigt und zum Merksatz umgeschrieben. **Für beide:** ein `kubectl port-forward` überlebt den Pod-Wechsel nicht — nach jedem Deploy `make port-forward` neu starten. Und: eine `401` auf `/api/...` sagt nichts darüber, ob eine Route existiert; die Auth-Middleware liegt vor dem inneren Mux. |
| 2026-08-31 | Claude Code | **Skalierungsziel als F2 aufgenommen — nichts implementiert.** Marco nennt die Beta als Zwischenstand und peilt ~10.000 registrierte Benutzer bei ≥500 gleichzeitigen an. Am Code nachgesehen, was zuerst bricht (F2.1): Zwei-Benutzer-Trigger, Argon2id mit 64 MiB und genau zwei Slots, DB-Pool 5 bei einem Session-Lookup je Anfrage, Rate-Limiter als Map im Prozessspeicher **ohne Eviction** (pro Replikat wirkungslos und ein Speicherleck), Belege auf einem `ReadWriteOnce`-PVC mit 1 GiB, `homeflow_expenses_due` als Full Scan bei jedem Scrape. Der eigentliche Kern ist F2.2: es gibt **kein Mandantenkonzept** — die Datenbank *ist* der Haushalt, und `visibility='shared' OR owner_id=$1` im Kalender würde mit 10.000 Benutzern jeden geteilten Termin offenlegen. Deshalb `household_id` plus Row Level Security **vor** allem anderen, und in F1 von der ersten Migration an. Zielarchitektur bleibt der modulare Monolith, nur zustandslos (F2.3), acht Etappen mit Lasttest als Abnahme (F2.4). **Konflikt ausdrücklich benannt, nicht entschieden:** Zwei-Benutzer-Grenze, Kein-Remote-Access und die 0-Euro-Regel tragen dieses Repo; F2.5 stellt Weg A (lokal, 0 €) gegen Weg B (Betrieb mit laufenden Kosten, Mailversand für Registrierung, DSGVO-Pflichten) und verlangt für B neue ADRs statt stillschweigender Brüche. F2.6 macht „komfortabel und intuitiv" konkret, F2.7 listet, was unabhängig von der Entscheidung sofort sinnvoll ist. Zwei bei der Analyse gefundene **heutige** Defekte stehen zusätzlich als Beobachtung 7.10 (Rate-Limiter räumt nur nach erfolgreichen Anmeldungen auf — unbegrenztes Wachstum, pro Replikat wirkungslos) und 7.11 (`homeflow_expenses_due` fragt bei jedem Scrape die Datenbank) in Abschnitt 7, damit sie nicht nur in einem Zukunftskapitel liegen; Querverweise auf F2 in 4.4 und 5.2 ergänzt. Außerdem eine **Korrektur an meinem eigenen F1.6**: Dokumente landen auf dem 1-GiB-Beleg-PVC, nicht auf 3 GiB — also rund 100 statt 300 Dateien in Maximalgröße. |
| 2026-08-31 | Claude Code | **Pet-Modul vorgeprüft und als F1 im Backlog abgelegt — nichts implementiert.** Marcos Vorlage gegen Schema und Code geprüft: der sichere Dokumentpfad (Magic Bytes, serverseitiger Speicherschlüssel, `attachment`, Content-Type-Pinning, Traversal-Schutz) existiert seit P2-2 fertig und wird geerbt; Tiertermine passen in `calendar_events` statt in eine eigene Tabelle; die Zwei-Benutzer-Grenze ist nicht berührt. Vier Stellen passen nicht und stehen in F1.2: `receipts.expense_id` ist `NOT NULL` (ein Laborbericht hat keine Rechnung), es gibt nichts, was zu einem Zeitpunkt etwas tut (kein Scheduler, kein Kanal, `event_outbox` ohne Schreiber → Vorschlag: Erinnerungen beim Lesen berechnen statt zustellen), „mehrere Tiere je Artikel" ist n:m und braucht eine Verknüpfungstabelle, und P2-5 wird vom Nice-to-have zur Voraussetzung. Zuschnitt-Vorschlag: 8 neue Tabellen statt der 16 aus der Vorlage, Akte als **eine** Zeitleisten-Tabelle mit Typ. Zehn Etappen, je einzeln lauffähig, Migrationsnummern bewusst **nicht** auf Vorrat reserviert. Kosten 0 €, RAM unverändert; die einzige echte Größe ist das 3-GiB-PVC (rund 300 Dokumente in Maximalgröße). Fünf Entscheidungen liegen bei Marco (F1.5). Keine Code-, Schema- oder OpenAPI-Änderung in diesem Durchgang. |
| 2026-08-31 | Claude Code | **P2-3 Kalendermodul.** Neues Paket `internal/calendar` (Model, Service, PostgreSQL- und Memory-Repository) im Aufbau der drei bestehenden Module; `internal/domain/calendar/model.go` ist damit entfallen. Fünf Routen: `GET/POST /api/v1/calendar/events` und `GET/PUT/DELETE /api/v1/calendar/events/{eventId}`, zuerst in der OpenAPI, dann implementiert, 4.1 nachgezogen. **Verdrahtung, die dich betrifft:** der Dienst hängt an einem neuen Feld `Calendar` in `httpapi.BrowserCookieConfig` — derselben Struktur, in der auch dein `Metrics` steckt; ist es `nil`, werden die Routen gar nicht erst registriert (wie bei Ausgaben und Belegen). Die Signatur von `NewFullWithOptions` ist dadurch unverändert geblieben. Keine Migration nötig — Tabelle und Index existierten. **Sichtbarkeit ist die erste echte Autorisierungsregel im Repo:** geteilte Termine gehören dem Haushalt und sind von beiden bearbeitbar, private nur der Eigentümerin; der Zugriff der anderen Person endet in 404 statt 403, damit die Existenz nicht bestätigt wird. Die Regel steht in der WHERE-Klausel jeder lesenden und schreibenden Anweisung, nicht nur im Service. Eigentümer und `created_at` sind bei `PUT` unveränderlich; `ownerId` im Body wird durch `DisallowUnknownFields` mit 400 abgelehnt. **Bewusst nicht umgesetzt:** RRULE wird gespeichert, aber nie expandiert (so verlangt es `domain-model.md`) — ein wöchentlicher Termin ist eine Zeile, kein Serienlauf; `reminderMinutes` wird gespeichert, aber von nichts zugestellt (kein Scheduler, `event_outbox` hat weiterhin keinen Schreiber); kein iCal-Export. Der Listen-Endpunkt verlangt `from`/`to` (RFC 3339 oder Datum) und deckelt den Zeitraum bei 366 Tagen; geliefert wird nach Überschneidung, nicht nach Startdatum, damit ein über die Monatsgrenze laufender Termin nicht verschwindet. UI: dritter Reiter „Kalender" mit Monatswahl, Gruppierung nach Tag, Ganztägig-Umschaltung, Privat-Markierung, Erledigt/Wieder öffnen und Löschen (`App.tsx`, `api.ts`, Zeitumrechnung in `format.ts`, `.day`/`.event-list`/`.inline`/`.tag.private` in `styles.css`). Verifiziert: `make ci` grün (43 Frontend-Tests, neue Go-Tests für Service, HTTP-Schicht und Sichtbarkeit); `make integration-test` gegen ein Wegwerf-PostgreSQL — die Sichtbarkeits-SQL, `NULLIF` für die leere RRULE und die Überschneidungsabfrage sind gegen das echte Schema geprüft; dazu ein Ende-zu-Ende-Lauf mit **zwei** angemeldeten Benutzern: der private Termin taucht in Liste, GET, PUT und DELETE der anderen Person nirgends auf (3× 404), der geteilte ist von beiden bearbeitbar und behält seinen Eigentümer, PUT/DELETE ohne `X-CSRF-Token` sind 403, Randfälle 401/400/400/400/422/400, kein einziger 500er. **Nicht geprüft:** kein Klick-Durchlauf im Browser. **Für Codex:** keine neue Metrik, aber siehe Beobachtung 7.7 — das `route`-Label trifft alle `/api/`-Routen gleich, auch die neuen. |
| 2026-08-31 | Claude Code | **P2-2 Belege ansehen.** Neu: `GET /api/v1/expenses/{id}/receipts/{receiptId}/content` — immer `attachment`, Content-Type auf die drei beim Upload per Magic Bytes erkannten Typen gepinnt (alles andere wird `application/octet-stream`), zusätzlich `Content-Security-Policy: default-src 'none'; sandbox` je Antwort. Der Speicherschlüssel bleibt serverseitig; adressiert wird über die Beleg-ID, und die SQL-Abfrage verlangt zusätzlich die passende Ausgabe, ein Beleg ist also nicht über eine fremde Ausgabe lesbar. `LocalStorage.Open` validiert den Schlüssel erneut gegen Traversal und akzeptiert nur reguläre Dateien. Neu außerdem `id.IsUUID`: kaputte Pfad-IDs werden 400 statt bisher 500 (betraf auch die schon vorhandene Beleg-Liste). Dateiname im Header über `mime.FormatMediaType`, mit neutralem Rückfall. Frontend: die Aufklappzeile heißt jetzt „Belege", lädt die Liste beim Öffnen nach und verlinkt jeden Beleg (`api.ts`, `App.tsx`, `.receipt-list` in `styles.css`). OpenAPI in 4.1 nachgezogen. Verifiziert: `make ci` grün (25 Frontend-Tests, neue Go-Tests für Download, Header, Traversal, fremde Ausgabe, fehlende Auth); `make integration-test` gegen ein Wegwerf-PostgreSQL (Container danach entfernt) — die neue SQL-Abfrage samt Ausgaben-Bindung ist gegen ein echtes Schema geprüft; zusätzlich ein vollständiger Ende-zu-Ende-Lauf gegen eine eigene Wegwerf-Instanz: Login, Ausgabe, Bezahlung mit abweichendem Ist-Betrag (Übersicht liefert `planActualDeltaCents: -825`, P1-1 damit auch am echten Backend belegt), Upload mit Umlaut-Dateinamen, Download byte-identisch, `filename*=utf-8''Stromrechnung%20M%C3%A4rz.pdf`, und die Negativfälle 401/404/404/400/400/400 ohne einen einzigen 500er. **Nicht geprüft:** kein Klick-Durchlauf im Browser — dafür bräuchte es eine Anmeldung. **Für Codex:** `mime_type` ist in Migration `000001` per CHECK auf dieselben drei Typen begrenzt; wer die Liste erweitert, fasst Migration, `detectType` und `pinnedMIMEType` zusammen an. |
| 2026-08-31 | Claude Code | **P1-5 `make ci` + optionaler pre-push-Hook.** Neu: `make fmt-check` (meldet gofmt-Abweichungen, schreibt nichts), `make ci` (fmt-check → lint → test → `kubectl kustomize` für `kubernetes/base` **und** `monitoring/kubernetes`) und `make install-git-hooks`. Hook-Skript `scripts/pre-push.sh`, Installer `scripts/install-git-hooks.sh`. **Der Hook ist nicht installiert** — `.git/hooks/` ist unverändert leer; Installation nur auf ausdrücklichen Wunsch. Verifiziert: `make ci` läuft grün durch; mit absichtlich verformatierter Go-Datei bricht `fmt-check` und damit `ci` mit Exit≠0 ab (Datei danach zurückgespielt); Installer in einem Wegwerf-Repo gegen alle Pfade getestet (Installation, wiederholte Installation, Verweigerung bei fremdem Hook, `--force` mit `.bak`, `HOMEFLOW_SKIP_CI=1`, Abbruch bei fehlschlagendem `ci`). **Nicht in `ci`:** `make integration-test` (braucht `HOMEFLOW_TEST_DATABASE_URL`) und `make build`. `ci` setzt `kubectl` voraus. Da das Repo kein Remote hat, ist ein pre-push-Hook bis auf Weiteres schlafend (P3-4). |
| 2026-08-31 | Claude Code | **P1-1 Ist-Betrag im UI.** `ExpenseRow` hat jetzt ein Eingabefeld „Ist-Betrag in €", vorbelegt mit dem Planbetrag; „Als bezahlt markieren" schickt den eingegebenen Wert statt blind `plannedAmountCents`. Weicht der Ist-Betrag ab, steht der Planbetrag als Kontext in der Zeile. Neu in `format.ts`: `amountInputValue` und `centsFromAmountInput` — die Umrechnung läuft string-basiert, damit „1,15" 115 Cent ergibt und nicht 114. Ungültige Eingaben werden vor dem Request abgefangen und als Meldung angezeigt. `styles.css` um die Klasse `.pay` ergänzt. Backend und OpenAPI unverändert: `MarkPaid` akzeptiert jeden Wert ≥ 0. Verifiziert: `make frontend-test` (22 Tests, davon 20 neu, inkl. Render-Test von `ExpenseRow` über `react-dom/server` — keine neue Abhängigkeit), `make lint` und `make frontend-build` grün. **Nicht geprüft:** kein Klick-Durchlauf im Browser — dafür bräuchte ich Anmeldedaten. `ExpenseRow` ist für den Test exportiert. |
| 2026-08-31 | Claude Code | **Fremdarbeit nachgeprüft (Alarmregeln + NetworkPolicy).** NetworkPolicy: Beobachtung 7.2 war **falsch** und ist korrigiert — kindnet erzwingt die Policies auf diesem Cluster, empirisch belegt (Tabelle in 7.2). Die Policies sind jetzt angewendet, die irreführende WARNUNG im Kopf von `kubernetes/base/networkpolicy.yaml` ist ersetzt. Selektoren stimmen gegen die realen Pod-Labels, der erlaubte Pfad API→PostgreSQL funktioniert auch nach `rollout restart`. Alarmregeln: `kubectl kustomize monitoring/kubernetes` rendert; die ConfigMap enthält zehn eindeutig benannte Regeln in vier Gruppen mit vollständigen Feldern; `rule_files` zeigt auf `/etc/prometheus/alerts.yml`, was zum ConfigMap-Mount passt; alle referenzierten Metriknamen, Labels und Suffixe (`_bucket`, `_count`) stimmen gegen `metrics.go`, `histogram_quantile` behält überall `le`. **Nicht geprüft: die PromQL-Semantik** — kein Parser verfügbar, `promtool` fehlt lokal und der Stack läuft nicht; liegt laut Marco bei Codex. Ebenfalls ungeprüft: der Prometheus-Scrape gegen `api-ingress-monitoring`, weil es keinen Prometheus gibt. Zwei inhaltliche Anmerkungen für Codex: `HomeflowDbErrors` (`rate[5m] > 0` `for: 10m`) verlangt **dauerhafte** Fehler, ein einzelner Fehler löst nie aus — passt zur Beschreibung, ist aber leicht misszuverstehen; `HomeflowTargetMissing` und `HomeflowApiDown` feuern bei API-Ausfall gemeinsam. |
| 2026-08-31 | Claude (Cowork) | **`AGENTS.md` und `CLAUDE.md` angelegt.** Dauerhafte Arbeitsanweisung (goldene Regeln, Session-Ablauf, Verifikationspflicht je Verzeichnis, Sicherheitsgrenzen, Sprachkonvention, bekannte Fallen). Codex liest `AGENTS.md` automatisch, Claude Code liest `CLAUDE.md`, das per `@AGENTS.md` darauf verweist — eine Quelle, zwei Einstiegspunkte. Diese Datei hier bleibt der laufende Terminkalender; `AGENTS.md` ändert sich selten. |
| 2026-08-31 | Claude (Cowork) | **P1-2 Alarmregeln.** Zehn Regeln in vier Gruppen als Key `alerts.yml` in der bestehenden ConfigMap `prometheus-config`, plus `rule_files` in `prometheus.yml`. Rein additiv: genau zwei Einfügepunkte in `monitoring/kubernetes/prometheus.yaml`. Kein Alertmanager (RAM, s. 5.1) — Alarme sind unter `/alerts` sichtbar, werden aber nicht zugestellt. `monitoring/README.md` ergänzt. **Achtung Codex:** beides liegt in deinem aktiven Claim-Scope; ich habe es auf Marcos ausdrückliche Anweisung geändert und den Fußabdruck bewusst minimal gehalten. Verifiziert: YAML-Rundlauf und Abgleich aller referenzierten Metriknamen gegen `metrics.go`. **Nicht verifiziert:** PromQL-Semantik — bitte einmal `promtool check config` laufen lassen (Befehl steht in `monitoring/README.md`). |
| 2026-08-31 | Claude (Cowork) | **P1-4 NetworkPolicy.** `kubernetes/base/networkpolicy.yaml` neu (default-deny, DNS, API→PostgreSQL, Prometheus→`/metrics`), in `kustomization.yaml` aufgenommen. Selektoren maschinell gegen die realen Pod-Labels im Repo geprüft. **Wirkungslos unter kindnet** — siehe Beobachtung 7.2; der Namespace `homeflow-monitoring` ist bewusst ausgespart, weil Prometheus' Egress zum API-Server CNI-abhängig ist. |
| 2026-08-31 | Claude (Cowork) | **P1-3 Doku-Drift.** `README.md` auf Sprint 5/0.5.0, neue Abschnitte „Monitoring" und „Netzwerkisolation" inkl. Nachweisbefehl; `docs/architecture/overview.md` um Sprint-5-Observability und Netzwerkisolation ergänzt; `docs/security/threat-model.md` um Sprint-5-Kontrollen, die Begründung für unauthentifiziertes `/metrics` und den fehlenden CNI-Enforcement-Abschnitt erweitert. |
| 2026-08-31 | Claude (Cowork) | Backlog (Abschnitt 6) aus vollständiger Lückenanalyse ergänzt; Abschnitt 2 „Umgebungen" neu — Claude via Cowork kann Go/TS **nicht** bauen oder testen, deshalb gehören `backend/`- und `frontend/`-Änderungen zu Codex oder Claude Code auf dem Laptop. Goldene Regeln (Dokumentationspflicht, Nur-kostenlos) vorangestellt. Keine Codeänderung. |
| 2026-08-31 | Claude (Cowork) | `Austausch.md` angelegt: Claim-Verfahren, Schnittstellenregister (API, Metriknamen, Migrationsnummern, ENV, Cookies, Version), Mengengerüst und offene Beobachtungen. Keine Codeänderung. |
