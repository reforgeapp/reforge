# Local development

Foundation commands: `npm --prefix web ci`, `make generate`, `make build`, `make check`, `make test`.

Requires Go 1.27.1 (automatic toolchain download), Node 26 and PostgreSQL 18.6. Dependency versions are pinned in the module and npm lockfiles. Standard Go/npm dependency installation needs public registry access.

Apply SQL with `REFORGE_MIGRATION_DATABASE_URL` set to the offline schema-owner connection with BYPASSRLS for tenant backfills and `make migrate`. Runtime uses `REFORGE_DATABASE_URL` with a separate non-owner, non-superuser, non-BYPASSRLS role. The migration role is never granted to the runtime. Grant table/sequence privileges after migration; never provide migration credentials to the running server.

With roles named `reforge_migrator` and `reforge_runtime`, apply `scripts/runtime-grants.sql` as the migration owner after migration. Audit events are append-only for the runtime. Neither role is a superuser; the runtime cannot create schema objects, own the database or inherit the migration role.

Set `REFORGE_ENCRYPTION_KEY` to an operator-owned random 32-byte base64 key. Preserve it with encrypted backups. `make dev` explicitly opts into loopback-only fixture authentication; production refuses that mode on public addresses and in hosted edition. The frontend displays fixture status from `/api/v1/meta`.

`make test-integration` uses an explicitly supplied disposable test database. `make test-e2e` uses Playwright against the local application. Qualification targets fail when their prerequisite is absent; they never report skipped checks as success.

This workspace has disposable PostgreSQL 18.6 on `127.0.0.1:55432`. Local credentials/key live in ignored mode-0600 `.local/development.env`; load with `set -a; source .local/development.env; set +a`. Database names are `reforge_dev` and `reforge_test`. The test runner validates both database URLs identify the same `reforge_test` database before applying migrations. `/tmp/reforge-postgres/bin/pg_ctl -D .local/postgres status` checks the local service; `stop -m fast` stops it. These paths describe this implementation environment, not the distributable install.

