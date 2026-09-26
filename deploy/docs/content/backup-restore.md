# Backup and restore

Two things must be backed up together: the database and the credential encryption key
material.

## What to back up

- The PostgreSQL database (migration history, tenants, connections, findings, runs,
  changes, deployments, campaigns, audit).
- The encryption key material: the local envelope key, or the KMS key identifiers and the
  policy that allows decrypt. Losing the key makes stored credentials unrecoverable.
- The deployment environment file, stored in your secret manager, not in the image.
- The artifact store (the `artifacts` Compose volume or the `reforge-artifacts` PVC), which
  holds run logs and repair evidence. Back it up with the database so records and payloads
  match.

## Restore

1. Restore the database into an empty PostgreSQL instance.
2. Provide the same encryption key material through the environment or KMS.
3. Start the migrator and confirm no pending migrations.
4. Start the control plane and verify a connection test decrypts an existing credential.

If credentials cannot be decrypted, the restore is incomplete: re-enter the affected
connection secrets rather than deleting the records.

## Drill

`make restore-drill` (or `scripts/restore-drill.sh`) dumps a source database, restores it
into a scratch database and verifies the schema, migration count and encryption-key
recovery. It needs `REFORGE_DRILL_SOURCE_URL`, a superuser `REFORGE_DRILL_ADMIN_DB`, a
scratch `REFORGE_DRILL_TARGET_URL`, the backed-up `REFORGE_ENCRYPTION_KEY`, and PostgreSQL
client binaries on `REFORGE_DRILL_PG_BIN`. With `REFORGE_DRILL_ARTIFACTS` set to a restored
artifact directory, it also checks every unexpired artifact record has its payload. Run it against a disposable copy, never
production. A successful drill restores the schema and decrypts a sealed envelope; it does
not prove provider credentials still work, which needs a live connection test.

## Retention

Audit retention is an operator setting. The server deletes expired artifacts hourly, so they
no longer count towards the tenant quota. Export audit to durable storage before
shortening retention. Restoring a backup restores the audit history as of the backup.
