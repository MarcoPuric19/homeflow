#!/usr/bin/env sh
set -eu

if [ "$#" -ne 2 ] || [ "$1" != "--confirm-destructive-restore" ]; then
  echo "Refusing restore. Usage: $0 --confirm-destructive-restore <backup-directory>" >&2
  exit 2
fi

backup_dir=$2
./scripts/verify-backup.sh "${backup_dir}"

echo "This replaces the configured HomeFlow database and receipt directory." >&2
echo "Stop the API before continuing. Press Ctrl-C now to abort." >&2
sleep 10

docker compose exec -T postgres dropdb -U "${POSTGRES_USER}" --if-exists "${POSTGRES_DB}"
docker compose exec -T postgres createdb -U "${POSTGRES_USER}" "${POSTGRES_DB}"
docker compose exec -T postgres pg_restore -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" --clean --if-exists <"${backup_dir}/postgres.dump"
receipt_root=${HOMEFLOW_RECEIPT_STORAGE:-./storage/receipts}
mkdir -p "${receipt_root}"
tar -C "${receipt_root}" -xzf "${backup_dir}/receipts.tar.gz"

echo "Restore completed. Start HomeFlow and perform application-level verification."
