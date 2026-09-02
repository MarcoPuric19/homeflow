# Sicherheit

*English: HomeFlow runs only on loopback on a private laptop. There is no
public instance. Please report findings via GitHub's private vulnerability
reporting; see the scope notes below.*

## Wie melden

Bitte über **GitHub Security Advisories** („Report a vulnerability" im Reiter
*Security*), nicht über ein öffentliches Issue. Ein öffentliches Issue macht
den Fund lesbar, bevor er behoben ist.

Eine Rückmeldung kommt, sobald ich sie schreiben kann — das hier ist ein
privates Projekt von zwei Personen, keine 24/7-Bereitschaft. Feste
Reaktionszeiten sage ich deshalb nicht zu.

## Was HomeFlow ist, bevor du Zeit investierst

HomeFlow läuft **ausschließlich auf Loopback** auf einem privaten Laptop. Es
gibt **keine öffentliche Instanz**, keinen Ingress, keinen Tunnel, keine
DNS-Einträge und keine erreichbare Test-Umgebung. Wer hier einen Host findet,
der auf HomeFlow hört, hat nicht dieses Projekt gefunden.

Daraus folgt für einige übliche Befunde:

- **Fehlendes TLS / `Secure`-Cookie-Flag.** `HOMEFLOW_COOKIE_SECURE=false` ist
  ein bewusster, dokumentierter Loopback-Kompromiss. Er ist im Bedrohungsmodell
  als Bedingung hinterlegt, die erfüllt sein muss, bevor irgendetwas über
  Loopback hinausgeht. Kein Befund, solange das gilt.
- **Fehlende Ratenbegrenzung an der Anmeldung** bei zwei administrativ
  angelegten Benutzern ohne öffentliche Registrierung: bekannt und bewertet.
- **Rollen ohne Wirkung.** `role` wird gespeichert, aber von keiner
  Autorisierungsprüfung ausgewertet. Das steht so in
  [`docs/security/threat-model.md`](docs/security/threat-model.md) — bekannt,
  kein neuer Fund.

Was dagegen sehr willkommen ist: alles, was auch unter diesen Annahmen trägt —
Umgehung der Authentifizierung, SQL-Injection, Pfadausbrüche beim Umgang mit
hochgeladenen Belegen, Fehler in der Container- oder NetworkPolicy-Absicherung,
Secrets, die in Logs oder Metriken landen.

## Was hier nicht liegt

Keine Zugangsdaten, keine Tokens, keine Belege, keine Haushalts- oder
Tierdaten. `.env`, lokale Secret-Overlays, `storage/receipts/` und `backups/`
sind ignoriert und bleiben es. Sollte doch etwas davon im Repository landen,
ist genau das ein meldenswerter Fund.

## Bedrohungsmodell

[`docs/security/threat-model.md`](docs/security/threat-model.md) benennt
ausdrücklich auch die Wege, die **nicht** geschützt sind, samt der Bedingungen,
die vor einer Öffnung nach außen erfüllt sein müssten.
