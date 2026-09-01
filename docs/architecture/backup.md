# Backup plan

Version 1 backups remain local and explicit:

- PostgreSQL: timestamped `pg_dump --format=custom`
- receipts: filesystem snapshot/archive preserving checksums
- configuration: non-secret manifests from Git plus a separately encrypted
  offline copy of required secrets

Backups must not share the only physical disk indefinitely. A later runbook will
define encrypted removable-media copies, retention and quarterly restore tests.
No cloud backup is assumed.

## Commands

```bash
make backup
make verify-backup BACKUP_DIR=backups/<timestamp>
```

Backups are created with mode-restrictive defaults and contain a PostgreSQL
custom-format dump, receipt archive, configuration template and SHA-256
manifest. They still contain highly sensitive household data and are not
encrypted automatically.

Restore is intentionally not a Make target. It replaces database and receipt
state and requires the explicit marker below:

```bash
./scripts/restore.sh --confirm-destructive-restore backups/<timestamp>
```

Never execute that command against the main environment merely to test it.
Restore tests use a temporary `_test` database/container and a temporary receipt
directory. At least quarterly, verify dump restoration and application-level
counts/checksums on separate local media.
