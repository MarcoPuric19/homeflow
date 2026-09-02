## Was ändert sich

<!-- Ein bis drei Sätze. Der Titel landet so in der Release-Notiz. -->

## Wie geprüft

<!--
Konkret: welche Befehle, welches Ergebnis. `make ci` deckt gofmt, vet, Tests,
beide Manifest-Renderings, Migrationsnummern, den Ressourcenkontrakt und die
Alarmregeln ab.
-->

- [ ] `make ci` läuft durch
- [ ] `make ci-full` (falls Migrationen oder Datenbankcode betroffen sind)

## Was ich nicht prüfen konnte

<!--
Ausdrücklich benennen statt weglassen. Ein „müsste funktionieren" ohne Hinweis
darauf, was ungeprüft blieb, ist der eigentliche Fehler.
-->

## Verträge

- [ ] Schnittstellen, Mengen oder Migrationsnummern geändert → `docs/architecture/contracts.md` im selben Durchgang angepasst
- [ ] Keine Zugangsdaten, Tokens oder privaten Daten in Code, Logs, Metriken oder Commit-Nachrichten
