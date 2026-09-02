#!/usr/bin/env sh
# Startet eine Wegwerf-PostgreSQL, führt einen Befehl dagegen aus und räumt sie
# in jedem Fall wieder ab.
#
# AGENTS.md beschreibt das Rezept seit Langem als Handarbeit. Allein am
# 2026-09-01 habe ich es viermal von Hand getippt, und `make ci` enthält die
# Integrationstests bewusst nicht, damit es ohne Datenbank durchläuft. Folge:
# `make integration-test` fährt nur, wer daran denkt - und Migrationen sind
# genau der Bereich, in dem ein Fehler still bis in die Daten durchschlägt
# (HF-0097).
#
# Der Container lebt nur während des Laufs. Das Image liegt für den Cluster
# ohnehin lokal, kostet also nichts Dauerhaftes.
set -eu

: "${PG_IMAGE:=postgres:17-alpine}"
: "${PG_DB:=homeflow_test}"
: "${PG_PORT:=}"

[ $# -ge 1 ] || { echo "Aufruf: $0 <befehl> [argumente...]" >&2; exit 64; }
command -v docker >/dev/null 2>&1 || { echo "docker fehlt" >&2; exit 69; }

case "$PG_DB" in
  *_test) ;;
  # make integration-test verweigert ohnehin alles andere; hier scheitert es
  # früher und mit einem Satz, der sagt warum.
  *) echo "PG_DB muss auf _test enden, ist aber '$PG_DB'" >&2; exit 64 ;;
esac

# Freien Loopback-Port suchen statt einen festen zu belegen: auf diesem Laptop
# läuft oft schon eine Datenbank, und zwei Läufe gleichzeitig sollen sich nicht
# gegenseitig abschießen.
if [ -z "$PG_PORT" ]; then
  PG_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
fi

NAME="homeflow-inttest-$$"
PASSWORD=$(openssl rand -hex 16)

# Aufräumen auch bei Strg-C und bei Fehlschlag. Ein zurückgelassener Container
# belegt Port und Speicher, und der Speicher ist auf dieser Maschine knapp.
aufraeumen() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap aufraeumen EXIT INT TERM

docker run -d --rm --name "$NAME" \
  -e POSTGRES_PASSWORD="$PASSWORD" -e POSTGRES_DB="$PG_DB" \
  -p "127.0.0.1:$PG_PORT:5432" "$PG_IMAGE" >/dev/null

# pg_isready allein reicht als Wartebedingung NICHT: PostgreSQL meldet sich
# schon während der initdb-Phase bereit und startet danach neu. Der erste
# Connect läuft dann in ein "connection reset by peer". Deshalb ein echter
# Verbindungsaufbau, so wie AGENTS.md es beschreibt.
i=0
while [ $i -lt 60 ]; do
  if docker exec "$NAME" psql -U postgres -d "$PG_DB" -c 'select 1' >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
  i=$((i + 1))
done
if ! docker exec "$NAME" psql -U postgres -d "$PG_DB" -c 'select 1' >/dev/null 2>&1; then
  echo "PostgreSQL im Container $NAME wurde nicht bereit" >&2
  docker logs "$NAME" 2>&1 | tail -20 >&2
  exit 69
fi

# Das Passwort steht nur in dieser Variablen und wird nirgends ausgegeben -
# weder in Logs noch in Fehlermeldungen (Sicherheitsgrenzen, AGENTS.md).
HOMEFLOW_TEST_DATABASE_URL="postgres://postgres:$PASSWORD@127.0.0.1:$PG_PORT/$PG_DB?sslmode=disable"
export HOMEFLOW_TEST_DATABASE_URL

"$@"
