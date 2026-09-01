# Mission Control 0.14.0

Lokales Cockpit für HomeFlow: zeigt, welche Pods wovon abhängen, und trägt den
Austausch zwischen den Assistenten als Tickets.

**Arbeitstitel-Status:** Mission Control ist noch nicht die verbindliche Quelle.
`Austausch.md` bleibt es, bis Marco die Umstellung ausspricht — siehe Ticket
MC-0002.

**Eigene Versionierung:** Mission Control zählt seine Versionen selbst und folgt
**nicht** der `VERSION` von HomeFlow. Es ist ein eigenes Produkt mit eigenem
Lebenszyklus; jedes Ticket trägt die Mission-Control-Version, unter der es
entstanden ist.

## Starten

`make install` richtet beides ein: den Autostart **und** den Menüeintrag.
Danach steht „Mission Control" mit eigenem Icon im Anwendungsmenü und öffnet
sich in einem eigenen Fenster.

```bash
make install    # systemd-Benutzerdienst + Desktop-Eintrag
make status     # läuft er?
make logs       # was macht er?
make stop       # Autostart wieder entfernen
```

Danach: <http://127.0.0.1:7777>

Ohne Autostart geht es auch: `make run` startet das Cockpit im Vordergrund.

Aus dem Repo-Wurzelverzeichnis geht dasselbe mit `make mission-control-install`,
`mission-control`, `-status`, `-logs` und `-stop`. `make ci` in der Wurzel führt
`mission-control-ci` mit aus, damit ein kaputtes Cockpit nicht erst auffällt,
wenn jemand es öffnet.

### Wie „Anwendung" hier gemeint ist

Der Menüeintrag startet `desktop/launch.sh`. Das Skript stellt sicher, dass der
Dienst läuft, wartet auf seine Bereitschaft und öffnet dann ein eigenes Fenster.

Mit Chromium, Chrome, Brave oder Edge wird der App-Modus benutzt: eigenes
Fenster, eigenes Icon in der Leiste, keine Adresszeile, keine Tabs. **Mit
Firefox geht das nicht** — der App-Modus wurde dort entfernt. Firefox bekommt
deshalb ein normales, eigenes Fenster; wer die Browserleisten trotzdem loswerden
will, nimmt im Menü den Eintrag „Ohne Browserleisten öffnen" (Vollbild, Ende mit
F11 oder Strg+W).

Ein natives GTK-Fenster wäre möglich, würde aber `cgo` und `webkit2gtk`
verlangen — also genau die Abhängigkeiten, die dieses Werkzeug bewusst nicht
hat. Deshalb der Umweg über einen vorhandenen Browser.

## Was es zeigt

Die Cluster-Ansicht liest je Namespace **einen** `kubectl`-Aufruf und leitet
daraus einen Abhängigkeitsgraphen ab. Jede Kante stammt aus einem echten
Objekt, nichts ist geraten:

| Kante | Woher |
| --- | --- |
| leitet zu | `service.spec.selector`, gegen die Pod-Labels aufgelöst |
| gehört zu | `ownerReferences` des Pods |
| hängt ein | `pod.spec.volumes` (PersistentVolumeClaim) |
| liest | Volumes, `envFrom` und `env.valueFrom` (ConfigMap/Secret) |
| darf erreichen | NetworkPolicy, Egress-Regeln |
| akzeptiert von | NetworkPolicy, Ingress-Regeln |

Ein Klick auf einen Kasten blendet alles Unbeteiligte aus und schreibt die
Abhängigkeiten daneben in Sätzen aus. Ein Service ohne passenden Pod und ein
Policy-Selektor ohne Treffer werden als auffällig markiert — beides sind
Ausfälle, die sonst still bleiben.

Die NetworkPolicies dieses Clusters **werden erzwungen** (kindnetd bringt
`kube-network-policies` mit, nachgewiesen in Ticket HF-0015). Die Kanten
sind also eine Kontrolle, keine Absichtserklärung.

## Log

Unter der Cluster-Übersicht, an Kibanas Discover angelehnt.

### Fünf Formate, ein Level

Der Parser kennt die Formate, die in diesem Cluster wirklich vorkommen — nicht
irgendwelche. Ohne diese Übersetzung gäbe es kein Level, und ohne Level keine
Farbe, keinen Filter und kein Histogramm:

| Quelle | Format | Beispiel |
| --- | --- | --- |
| HomeFlow-API | slog JSON | `{"level":"INFO","msg":"http request","status":200}` |
| Prometheus | logfmt | `time=… level=WARN msg="…" scrape_pool=…` |
| Grafana | logfmt | `logger=… t=… level=info msg="…"` |
| kube-state-metrics | klog | `I0831 22:34:48.305252 1 tls_config.go:357] "…"` |
| PostgreSQL | eigenes | `2026-09-01 06:30:38.389 UTC [68] LOG:  …` |

Alles andere bleibt sichtbar: unbekannte Zeilen werden als Text geführt, und
das Level wird notfalls aus Wörtern wie `panic` oder `error` geraten. Eine
Zeile, die das Werkzeug nicht versteht, wird nie verschluckt.

Die Levelnamen werden vereinheitlicht — PostgreSQLs `FATAL` und `PANIC`,
Grafanas kleingeschriebenes `info`, klogs `E`-Präfix landen alle bei
ERROR/WARN/INFO/DEBUG. Ein Filter wirkt dadurch über alle fünf Formate.

### Was die Ansicht zeigt

- **Level-Chips mit Zählern.** Klicken filtert. Die Zähler bleiben dabei über
  *alle* Zeilen berechnet: ein Filter, der die Fehler versteckt, darf nicht
  auch verstecken, dass es Fehler gibt.
- **Histogramm**, nach Level gestapelt, Fehler in Rot obenauf. Die schnellste
  Antwort auf „wann ist es passiert".
- **Feld-Seitenleiste** mit den häufigsten Werten je Feld — welche Statuscodes,
  welche Routen, welche Scrape-Pools. Ein Klick macht daraus einen Filter,
  bevor irgendjemand eine Abfrage tippen muss.
- **Aufklappbare Zeilen**: Feldtabelle plus die Rohzeile. Jedes Feld lässt sich
  von dort aus als Filter setzen.
- **Volltextsuche** über Nachricht und Rohzeile.
- **Zeitraum durch Ziehen im Histogramm.** Die schnellste Art zu sagen „zeig
  mir, was um diese Zeit passiert ist". Die Auswahl geht über die Balkenzeiten,
  die der Server geliefert hat, bedeutet also genau das, was die Balken zeigen.
  Der gewählte Zeitraum steht als Chip darüber und lässt sich mit einem Klick
  aufheben; ein Klick ohne Ziehen wählt nichts aus, damit ein Fehlklick nicht
  den Blick aufs Ganze löscht. Das obere Ende schließt den getroffenen Balken
  mit ein, die Grenze selbst ist ausschließend.
- Fehler und Warnungen haben farbigen Balken, farbiges Level und eingefärbten
  Text — sie stehen heraus, ohne dass man sie sucht.

Ausgewertet wird auf dem Server: 2000 Zeilen sind in Go billig und in
JavaScript teuer, und Histogramm und Zähler müssen zueinander passen.

`GET /api/logs` nimmt `level=ERROR,WARN`, `q=text`, beliebig viele
`feld=name:wert` sowie `von=` und `bis=` (RFC 3339 oder Millisekunden). Pod-, Container- und Namespace-Namen werden weiterhin gegen
das Kubernetes-Namensschema geprüft, bevor sie kubectl erreichen.

## Tickets

Das Vokabular ist an Jira angelehnt, weil es das ist, das wir alle schon kennen:
**Projekt, Typ, Priorität, Epic, Komponente, Version**.

| Feld | Werte |
| --- | --- |
| Projekt | `HF` (HomeFlow), `MC` (Mission Control) — der Schlüssel trägt es: `HF-0042`, `MC-0003` |
| Typ | Epic, Task, Bug, Beobachtung, Protokoll |
| Priorität | Hoch, Mittel, Niedrig |
| Version | **gefunden in**: die Release, auf der das Projekt beim Anlegen stand |
| Geliefert | **ausgeliefert in**: trägt erst ein erfolgreicher Deploy ein |
| Scope | Dateipfade oder Muster — siehe „Repo" |

Die beiden Versionsfelder sind bewusst getrennt. Sie zu einem zu verschmelzen
hat dazu geführt, dass 41 offene Tickets behaupteten, zu einer längst
ausgelieferten Release zu gehören — ein Ticket, das niemand angefasst hat, kann
nicht Teil einer fertigen Version sein. Was ausgeliefert wurde, weiß nur der
Deploy, und der trägt es ein.

`Protokoll` ist der Typ, den die übernommene Historie aus `Austausch.md`
bekommen hat: abgeschlossene Arbeit, die aufbewahrt wird. Solche Tickets sind
im Board ausgeblendet — sonst würden 28 fertige Einträge die Spalte „Fertig"
zumauern — und über „Protokoll zeigen" wieder einblendbar.

### Drei Ansichten

- **Board** — die fünf Spalten, gefiltert. Für die laufende Arbeit.
- **Backlog** — nach Epics gruppiert, darunter nach Typ. Für den Überblick über
  alles, was noch offen ist, und zum Anlegen neuer Tickets.
- **Versionen** — siehe unten.

Beide Listen teilen sich eine Filterleiste: Suche, Projekt, Typ, Zuständigkeit,
Version. Die Zeile darunter sagt immer, wie viele von wie vielen Tickets gerade
sichtbar sind, damit ein vergessener Filter nicht wie ein leeres Board aussieht.

### Historie übernommen

Am 2026-09-01 sind 53 Tickets aus `Austausch.md` entstanden: 2 Epics (F1, F2),
11 Backlog-Aufgaben, 12 Beobachtungen, 28 Protokolleinträge. Der Import ist
wiederholbar — `cmd/mcimport` gleicht über den Titel ab und überspringt, was es
schon gibt. Datum und Wortlaut bleiben unverändert; ein Importer, der Einträge
„verbessert", würde die Erinnerung des Projekts stillschweigend verändern.

```bash
go run ./cmd/mcimport -source ../Austausch.md -dry-run   # zeigt nur
```

## Repo

Die Ansicht **Repo** ist die Verbindung zwischen Tickets und dem Repository.
Sie besteht aus zwei Hälften, weil das Repository zwei Hälften Historie hat:

**Dateien — trägt heute.** Ein Ticket bekommt einen `scope`: Pfade oder Muster,
an denen es arbeitet. Drei Formen, in der Reihenfolge, in der man sie schreibt:

```
backend/internal/pets/      ein Verzeichnis - alles darunter
backend/**/*_test.go        ** überspringt Ebenen, * bleibt in einer
Makefile                    ein exakter Pfad
```

Daraus entstehen zwei Dinge:

- **Zuordnung:** jede unversionierte Änderung wird dem Ticket zugeschrieben,
  dessen Scope sie trifft. Was zu keinem passt, steht unter „Änderungen ohne
  Ticket" — sichtbar statt verschwiegen.
- **Kollisionswarnung:** zwei Tickets **in Bearbeitung** mit überlappenden
  Pfaden werden rot gemeldet. Das ist genau das, was die Claim-Tabelle in
  `Austausch.md` von Hand leistet, und es ist der Grund, warum ein Ticket in
  „In Bearbeitung" ein Claim ist und kein zweites Konzept nötig wird.

**Commits — trägt ab dem ersten Commit.** Ein Commit, dessen Betreff oder Rumpf
einen Ticketschlüssel enthält, gehört zu diesem Ticket:

```
git commit -m "HF-0042 Rollen wirksam machen"
```

Dieses Repository hat **keinen einzigen Commit** und kein Remote. Die Ansicht
sagt das ausdrücklich, statt eine leere Liste zu zeigen — es ist der Zustand,
den HF-0010 (P3-4) beschreibt, und er hat in dieser Session schon zweimal Arbeit
gekostet.

Mission Control ruft ausschließlich lesende git-Kommandos auf: `log`, `status`,
`rev-parse`, `remote`, `branch`. Es committet, staged und checkt nichts aus.

## Deploy

Die einzige Ansicht, die **etwas verändert**. Sie führt die make-Ziele dieses
Repos aus und zeigt die Ausgabe live mit.

| Ablauf | Schritte | Wirkung |
| --- | --- | --- |
| HomeFlow deployen | `ci` → `docker-build` → `kind-load` → `deploy` | ersetzt die laufende Anwendung |
| Monitoring deployen | `monitoring-manifests` → `monitoring-deploy` | wendet den Monitoring-Stack an |
| Mission Control bauen | `mission-control-ci` | baut das Cockpit selbst; der laufende Dienst bleibt unberührt bis zum Neustart |
| Nur prüfen | `ci` | ändert nichts |

### Ein Ablauf startet nur, wenn er etwas ändert

Ein Deploy, der nichts bewirkt, ist nicht harmlos: er ersetzt laufende Pods,
beendet offene Port-Forwards und kostet Minuten Bauzeit für nichts. Deshalb
prüft Mission Control **vor** dem Start, was sich überhaupt ändern würde:

| Prüfung | Womit |
| --- | --- |
| Version | `VERSION` aus der Makefile gegen das Image des laufenden Pods |
| Manifeste | `kubectl diff -k …` — kubectl selbst beantwortet die Frage |
| Cockpit | Quelltext gegen das gebaute Binary, **und** das Binary gegen den laufenden Dienst |

Findet sie nichts, ist der Knopf **für die Abläufe gesperrt, die ein laufendes
System verändern** — also für die beiden Cluster-Abläufe. Der Startversuch über
die API antwortet dort mit **412**.

Bauen und Prüfen sind davon ausgenommen: ein Lauf, der nur baut, kostet zwanzig
Sekunden und richtet keinen Schaden an. Ihn zu sperren hieße, jemanden
auszusperren, der genau das will.

Vom Diff werden nur die **Namen** der betroffenen Objekte angezeigt, nie der
Inhalt: der kann Konfigurationswerte enthalten, und das Cockpit hat nichts
damit zu tun, die auszudrucken.

**Es gibt einen Ausweg, und er heißt wie er ist.** Quelltext, der sich ohne
Versionssprung geändert hat, erzeugt denselben Tag und dasselbe Manifest — das
sieht die Prüfung nicht. „Trotzdem deployen" ist deshalb möglich, wird als
erzwungen protokolliert und steht so auch im Ticketkommentar.

### Was der Dienst nicht kann, und zwar absichtlich

- **Keine freien Kommandos.** Die API nimmt einen Pipeline-*Namen*, nie eine
  Kommandozeile. Die Liste steht im Code; sie zu erweitern ist eine
  Codeänderung.
- **Nur `kind-dev`.** Jeder Ablauf, der Kubernetes anfasst, liest vorher
  `kubectl config current-context` und weigert sich bei allem anderen — mit
  Nennung des gefundenen Kontexts.
- **Einer nach dem anderen.** Ein zweiter Durchlauf während eines laufenden
  würde sich um Image-Tag und Cluster streiten.
- **Nichts Fremdes.** Die Schritte sind make-Ziele, weil die Makefile die
  Schutzmechanismen schon trägt (`kind-load` verweigert einen fremden Kontext,
  `deploy` ein fehlendes Secret). Zwei Wahrheiten wären schlimmer als keine.
- **Kein Selbst-Deploy in einem Zug.** Der Ablauf „Mission Control bauen" legt
  nur ein neues Binary ab; übernommen wird es durch einen ausdrücklichen
  Neustart. Der antwortet zuerst und beendet sich dann - andersherum sähe die
  Oberfläche einen Verbindungsabbruch und könnte Erfolg nicht von Absturz
  unterscheiden.

Abschaltbar mit `--deploy=false`. Wer einen Ticketschlüssel angibt, bekommt den
Durchlauf als Kommentar an diesem Ticket protokolliert.

### Der Deploy schließt sein Ticket

Ein erfolgreicher Deploy zieht das angegebene Ticket von **Test** auf
**Fertig** — der Deploy ist die Abnahme, auf die diese Spalte wartet.

Bewusst eng gefasst, weil ein Board, das sich selbst schließt, weniger wert ist
als eines, das niemand pflegt:

- **Nur das Ticket, das beim Start genannt wurde.** Nicht alle auf Test: ein
  HomeFlow-Deploy sagt nichts über ein Ticket im Threat Model aus.
- **Nur aus der Spalte Test.** Wer in Backlog, Ready oder In Bearbeitung steht,
  wartet nicht auf Abnahme und wird auch nicht abgenommen.
- **Nur nach einem erfolgreichen Durchlauf.**
- **Nie nach „Nur prüfen".** Ein Lauf, der nichts ändert, nimmt nichts ab.
- **Immer mit Kommentar**, der Durchlauf und Grund nennt — und vermerkt, wenn
  der Lauf erzwungen war.

Weil längst nicht jede Arbeit in einem Cluster landet, zeigt die Ansicht
außerdem **„Wartet auf Abnahme"**: alle Tickets in der Spalte Test, und je
Ticket, ob ein Ablauf ihre Pfade überhaupt ausrollt. Ein Dokument, ein Skript
oder unverdrahteter Code bekommt dort ausdrücklich „kein Ablauf deckt diese
Pfade ab" statt stillschweigend zu fehlen — sonst wartet jemand auf einen
Knopf, der nie etwas tun wird.

### Herkunftsprüfung

Mission Control hat keine Anmeldung — es hört auf dem Loopback und vertraut
dem, der am Gerät sitzt. Ein **Browser** wird aber nicht nur von dieser Person
benutzt: jede beliebige Webseite kann den Browser dazu bringen, an
`127.0.0.1:7777` zu posten. Ohne Schutz hätte ein Seitenbesuch Tickets angelegt
und, seit es den Deploy-Dienst gibt, den Cluster neu gebaut.

Deshalb prüft jede **verändernde** Anfrage die Herkunft: ein fremder `Origin`
oder ein `Sec-Fetch-Site` ungleich `same-origin` wird mit 403 abgelehnt. Lesen
bleibt frei, und `curl` sowie Skripte senden keinen dieser Header und arbeiten
unverändert weiter.

## Versionen

Die Ansicht **Versionen** liest die Zahlen dort, wo sie wirklich stehen — nicht
aus einer gepflegten Liste:

| Gruppe | Quelle |
| --- | --- |
| HomeFlow | `Makefile`, `openapi.yaml`, `frontend/package.json`, laufendes Image im Cluster |
| Mission Control | die eigene Konstante |
| Im Cluster | die Images der laufenden Pods |
| Werkzeuge | `go.mod` beider Module, `kubectl version` |

Weicht eine der vier HomeFlow-Stellen vom Repository ab, wird sie rot markiert.
Genau dieser Fall ist zweimal aufgetreten (Cluster hinkt hinterher), und genau
deshalb steht er jetzt auf einer Seite statt in einem Kopf.

## Board-Spalten

Das Board hat fünf Spalten: **Backlog → Ready → In Bearbeitung → Test →
Fertig**. Die Spalten kommen aus der API (`GET /api/board`), damit Oberfläche
und Speicher nie unterschiedlicher Meinung darüber sind, was ein gültiger Status
ist. Ältere Statusnamen (`offen`, `in Arbeit`, `wartet`, `erledigt`) werden beim
Lesen auf die Spalten abgebildet, statt still zurückgesetzt zu werden.

Jedes Ticket trägt eine **Version** — die Mission-Control-Version, unter der es
entstanden ist. Neue Tickets bekommen die laufende Version, ältere ohne Feld
übernehmen sie beim nächsten Schreiben.

Ein Ticket ist eine Datei in `tickets/` — Markdown mit kurzem Kopf:

```markdown
---
id: MC-0001
projekt: MC
typ: Task
titel: Kurz und konkret
status: Backlog
prioritaet: Mittel
zustaendig: Codex
melder: Claude Code
version: 0.2.0
epic:
komponente: cluster
tags: deployment, cluster
erstellt: 2026-09-01T08:42:29+02:00
aktualisiert: 2026-09-01T08:42:57+02:00
---

Beschreibung.

## Kommentar — Codex · 2026-09-01T08:42:57+02:00

Antwort.
```

Das ist Absicht: beide Assistenten können Tickets mit ihren normalen
Dateiwerkzeugen lesen und schreiben, die Historie bleibt diff- und
commitfähig, und ein abgeschaltetes Cockpit blockiert niemanden. Unbekannte
Kopfzeilen werden ignoriert, ein von Hand angehängter Kommentar wird gelesen.

Status: `Backlog`, `Ready`, `In Bearbeitung`, `Test`, `Fertig`.

## API

| Route | Zweck |
| --- | --- |
| `GET /api/health` | Version und Zeitpunkt des letzten Snapshots |
| `GET /api/cluster` | letzter Snapshot (Knoten, Kanten, Warnungen) |
| `POST /api/cluster/refresh` | sofort neu einlesen |
| `GET /api/tickets[?status=&projekt=&typ=&zustaendig=&version=&epic=]` | Tickets, gefiltert |
| `POST /api/tickets` | anlegen |
| `GET /api/tickets/{id}` | einzelnes Ticket |
| `POST /api/tickets/{id}/comments` | kommentieren |
| `POST /api/tickets/{id}/status` | Status setzen |
| `POST /api/tickets/{id}/owner` | Zuständigkeit setzen |
| `POST /api/tickets/{id}/version` | Version setzen |
| `POST /api/tickets/{id}/priority` | Priorität setzen |
| `POST /api/tickets/{id}/type` | Typ setzen |
| `POST /api/tickets/{id}/epic` | einem Epic zuordnen |
| `POST /api/tickets/{id}/scope` | Datei-Scope setzen (leer = löschen) |
| `GET /api/repo` | Repo-Zustand, Zuordnung und Kollisionen |
| `GET /api/deploy` | Abläufe, laufender Durchlauf, Verlauf |
| `POST /api/deploy` | Durchlauf starten (`pipeline`, `author`, optional `ticket`, `trotzdem`) |
| `GET /api/deploy/changes` | was sich je Ablauf ändern würde |
| `POST /api/deploy/cancel` | laufenden Durchlauf abbrechen |
| `POST /api/deploy/restart` | Dienst neu starten, um ein neues Binary zu übernehmen |
| `GET /api/versions` | Versionen aller Plattformen |
| `GET /api/board` | Spaltenreihenfolge und laufende Version |
| `GET /api/logs?namespace=&pod=[&container=&lines=&level=&q=&feld=]` | ausgewertetes Log eines Pods |

## Grenzen, die bewusst so sind

- **Nur Loopback, keine Anmeldung.** Das Binary weigert sich zu starten, wenn
  `-addr` nicht auf `127.0.0.1` oder `localhost` zeigt. Wer das ändern will,
  braucht vorher eine Authentifizierung.
- **Fast nur lesen.** Mission Control ruft `kubectl get`, `kubectl logs` und
  lesende git-Kommandos auf. Die **einzige** Ausnahme ist der Deploy-Dienst,
  und der kann nur die drei Abläufe oben, nur gegen `kind-dev`.
- **Der beobachtete Cluster ist frei wählbar.** `-kubeconfig` und `-context`
  legen fest, wohin geschaut wird; Mission Control muss nicht in dem Cluster
  laufen, den es beobachtet, und hängt nicht an HomeFlows Lebenszyklus.
- **Keine Abhängigkeiten.** Kein npm, kein Build-Schritt, keine Go-Module außer
  der Standardbibliothek. Das Cockpit soll auch in einem Jahr noch bauen.
- **Kein Verlauf.** Es gibt nur den aktuellen Snapshot; Zeitreihen liefert
  Prometheus, siehe `monitoring/`.

## Ressourcen

Auf diesem Laptop gemessen (2026-09-01): der Dienst braucht rund **12 MiB
RSS**, der cgroup-Spitzenwert inklusive des kurzlebigen `kubectl`-Kindprozesses
liegt bei rund **24 MiB**. Die Unit deckelt bei 128 MiB. Ein Zyklus kostet
zwei `kubectl`-Aufrufe (je Namespace einen) alle 15 Sekunden; das Intervall
lässt sich mit `-interval` ändern.

Kosten: keine. Alles läuft lokal, Standardbibliothek, Open Source.
