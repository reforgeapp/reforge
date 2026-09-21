# Upgrade

Upgrades run migrations as a separate, one-shot step before the new control plane starts.

1. Back up the database and the encryption key material. See
   [Backup and restore](backup-restore.md).
2. Pull the new pinned images.
3. Run the migrator:

   ```sh
   docker compose --env-file .env -f deploy/compose/compose.yaml run --rm migrator
   ```

4. Restart the control plane, controllers and runner.
5. Check `/readyz` and the **Audit** route for the migration events.

## Failed migration

Migrations run in a transaction. A failed migration rolls back and leaves the applied
checksum history intact, so it can be retried after the cause is fixed. Do not edit an
applied migration; add a new one. If the schema is ahead of the binary, restore the
backup and deploy the matching image version.

## Runner rotation

Upgrade the runner by enrolling the new version and draining the old pool. Draining stops
new claims while in-flight work finishes; revoke the old runner after it is idle.
