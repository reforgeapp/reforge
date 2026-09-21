# Backup and restore

Two things must be backed up together: the database and the credential encryption key
material.

## What to back up

- The PostgreSQL database (migration history, tenants, connections, findings, runs,
  changes, deployments, campaigns, audit).
- The encryption key material: the local envelope key, or the KMS key identifiers and the
  policy that allows decrypt. Losing the key makes stored credentials unrecoverable.
- The deployment environment file, stored in your secret manager, not in the image.

Artifacts are stored separately; back them up if run evidence must survive.

## Restore

1. Restore the database into an empty PostgreSQL instance.
2. Provide the same encryption key material through the environment or KMS.
3. Start the migrator and confirm no pending migrations.
4. Start the control plane and verify a connection test decrypts an existing credential.

If credentials cannot be decrypted, the restore is incomplete: re-enter the affected
connection secrets rather than deleting the records.

## Retention

Audit and artifact retention are operator settings. Export audit to durable storage before
shortening retention. Restoring a backup restores the audit history as of the backup.
