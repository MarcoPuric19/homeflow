# HomeFlow

Private, API-first Haushaltsplattform für genau zwei Benutzer. Modularer
Go-Monolith mit PostgreSQL, React/TypeScript-Oberfläche aus demselben Binary,
betrieben auf einem Laptop in einem lokalen Kubernetes-Cluster.

Keine Cloud, keine Telemetrie, keine öffentliche Erreichbarkeit. Das ist kein
Zwischenstand, sondern die Produktentscheidung — die Begründungen stehen als
Architecture Decision Records in [`docs/adr/`](docs/adr/).

> **Status:** Beta. Die Plattform läuft, wird täglich benutzt und ist bewusst
> nicht öffentlich erreichbar. Die Umstellung vom Haushalts- auf ein
> Tierbetreuungs-Produkt ist im Gange
> ([`docs/product/`](docs/product/pet-platform-v2.md)).

## Warum das Repository interessant sein könnte

Weniger wegen der Fachlichkeit, mehr wegen der Betriebsentscheidungen dahinter:

- **Das RAM-Budget ist gemessen, nicht geschätzt.**
  [`docs/operations/resource-budget.md`](docs/operations/resource-budget.md)
  hält cgroup-v2-Werte gegen die Requests aus den Manifesten. Ein Skript prüft
  im CI, dass die Summe aller Requests unter `allocatable` bleibt — die
  Kontrakte können nicht unbemerkt auseinanderlaufen.
- **Alarmregeln werden auf Wirkung getestet, nicht nur auf Syntax.**
  Anlass war eine Regel, die einwandfrei rendert und das Ereignis, für das sie
  geschrieben wurde, trotzdem nie gemeldet hätte. Seitdem laufen
  `promtool test rules` gegen aufgezeichnete Zeitreihen.
- **Der Deploy glaubt seinem eigenen Exitcode nicht.** Er vergleicht die
  Image-IDs auf dem Node und in den bereiten Pods, weil ein Deploy schon
  einmal grün gemeldet und nichts ausgeliefert hat.
- **Das Bedrohungsmodell benennt, was *nicht* geschützt ist.**
  [`docs/security/threat-model.md`](docs/security/threat-model.md) führt die
  offenen Wege mit auf, inklusive der Bedingungen, die erfüllt sein müssten,
  bevor irgendetwas über Loopback hinausgeht.
- **Zwei Benutzer sind eine Datenbankbedingung**, keine Konvention in der
  Anwendungsschicht.

## Architektur in einem Absatz

Ein Go-Binary liefert die JSON-API und die gebaute SPA aus. Fachlichkeit liegt
in Modulen unter `backend/internal/` mit je eigenem Service und
PostgreSQL-Zugriff; ein modularer Monolith, weil verteilte Systeme für zwei
Benutzer Kosten ohne Nutzen sind. Migrationen sind nummeriert und werden gegen
Kollisionen geprüft. Schnittstellen, Mengen und Budgets stehen als Kontrakt in
[`docs/architecture/contracts.md`](docs/architecture/contracts.md) und haben
mehrere Konsumenten — wer einen ändert, ändert alle im selben Durchgang.

```
backend/        Go-Monolith: API, Domänenmodule, Migrationen
frontend/       React/TypeScript-SPA, wird ins Binary eingebettet
kubernetes/     Kustomize-Manifeste für den lokalen kind-Cluster
monitoring/     Prometheus, Grafana, kube-state-metrics, Alarmregeln
missioncontrol/ Eigenes Ticket- und Deploy-Cockpit, Go ohne externe Pakete
docs/           ADRs, Bedrohungsmodell, Betriebsnotizen, Produktentwürfe
scripts/        Prüf- und Betriebsskripte, die auch im CI laufen
```

## Loslegen

Voraussetzungen: Go 1.26, Node 22, Docker, kubectl, ein kind-Cluster.

```sh
make setup      # Abhängigkeiten
make ci         # alle Prüfungen, ohne Datenbank
make ci-full    # zusätzlich Integrationstests gegen Wegwerf-PostgreSQL
make dev        # lokal starten
```

`make help` listet alle Ziele. `make ci` ist bewusst ohne laufende Datenbank
lauffähig — das ist eine Eigenschaft, keine Lücke. Dieselben Ziele laufen im
CI, damit lokales und entferntes Urteil nicht auseinanderdriften.

## Mission Control

`missioncontrol/` ist ein eigenständiges Cockpit für Tickets, Claims und
Deploys: ein Go-Modul **ohne eine einzige externe Abhängigkeit**, mit
dateibasierten Markdown-Tickets und einer Oberfläche aus reinem JavaScript.
Es koordiniert die parallele Arbeit an diesem Repository, schreibt die
Release-Notizen aus den Ticketköpfen und prüft nach dem Deploy nach, ob
tatsächlich ausgeliefert wurde, was ausgeliefert werden sollte.

## Grenzen

- Erreichbar ausschließlich über Loopback. Kein Ingress, kein Tunnel, keine
  öffentliche Instanz.
- `HOMEFLOW_COOKIE_SECURE=false` ist ein bewusster Loopback-Kompromiss und
  dokumentiert; er muss `true` werden, bevor irgendetwas nach außen geht.
- Rollen werden gespeichert, aber noch von keiner Autorisierungsprüfung
  ausgewertet.
- Keine Zugangsdaten, Belege oder Haushaltsdaten im Repository — siehe
  [`SECURITY.md`](SECURITY.md).
- Die regulatorische Einordnung — NIS2, Cyber Resilience Act, DSGVO,
  Produkthaftung — steht mit Begründung **und Auslösern** in
  [`docs/legal/compliance.md`](docs/legal/compliance.md). Kurzfassung: nichts
  davon greift, und der Grund ist in drei von fünf Fällen derselbe.

## Lizenz

Apache-2.0 — siehe [`LICENSE`](LICENSE) und [`NOTICE`](NOTICE). Gewählt wegen
des ausdrücklichen Patentgrants und der Beitragsklausel, die eingehende
Änderungen ohne separates CLA unter dieselben Bedingungen stellt.

Sämtliche Fremdbestandteile mit ihren Lizenzen stehen in
[`docs/legal/dependencies.md`](docs/legal/dependencies.md) — sechzehn
Go-Module, alle permissiv, kein Copyleft im Binary. Dass diese Übersicht
aktuell bleibt, prüft `make check-licenses` bei jedem Lauf: eine neue
Abhängigkeit, die dort fehlt, lässt den Build scheitern.
