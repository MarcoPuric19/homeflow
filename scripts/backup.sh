#!/usr/bin/env sh
set -eu

umask 077
backup_root=${HOMEFLOW_BACKUP_DIR:-./backups}
receipt_root=${HOMEFLOW_RECEIPT_STORAGE:-./storage/receipts}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
destination="${backup_root}/${timestamp}"

mkdir -p "${destination}"
docker compose exec -T postgres pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" --format=custom >"${destination}/postgres.dump"
tar -C "${receipt_root}" -czf "${destination}/receipts.tar.gz" .
cp .env.example "${destination}/configuration.example"
(
  cd "${destination}"
  sha256sum postgres.dump receipts.tar.gz configuration.example >SHA256SUMS
)

echo "Backup created at ${destination}"
echo "It contains private household data. Move an encrypted copy to separate media."

