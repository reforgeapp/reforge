# T28a container install acceptance

Status: local self-hosted install, migration recovery, restart and encrypted restore passed. T28a release evidence remains partial.

Run artifacts: .local/t28a-install/t28a_20260923033417_26956/
Source revision: 6366f0355c4f83de00d5ed4e3b8b7e7ab449b288. Build ran with dirty worktree, including web/src/app/AppShell.tsx, web/src/styles/app.css and web/tests/mobile-navigation.spec.ts as concurrent uncommitted changes. Control image ID: sha256:e4920df37459a8d003d8c7e3578d29203ccbd14d1291bc7f264cf816a3ccb446. Harness cleanup removed image tags and project containers.

scripts/container-install-check.sh needs Docker Compose and shell utilities on host. It builds control and migrator images in Docker, runs Go helpers in pinned Go container, and runs PostgreSQL tools in PostgreSQL container. It uses no host Go, Node, Python, PostgreSQL client, HTTP client or /tmp path. Environment contains generated disposable passwords and known test-only encryption key.

| Check | Result | Evidence |
|---|---|---|
| Clean install | Pass; control/migrator images built; isolated migration/runtime roles and grants | build.log, clean-migration.log, roles.log, runtime-grants.log |
| Migration recovery | Pass; synthetic additive migration applied; injected failed DDL rolled back; retry and repeat succeeded | failed-upgrade.log, upgrade-retry.log, upgrade-idempotent.log |
| Production startup | Pass; development:false and fixture_auth:false; local HTTPS OIDC discovery, authorization and session | oidc-login.log, fixture/ca.pem |
| Process restart | Pass; /readyz returned ready after control process restart | harness assertion |
| Encrypted restore | Pass; database dump restored, migration check succeeded, stored secret envelope decrypted using backed-up key | backup.dump, restored-envelope.json, key-recovery.log, restore-migration.log |
| Restored service | Pass; control process ready against restored database; fixture OIDC session passed | restored-oidc-login.log |

Upgrade scenario uses synthetic migration with current migrator binary; not released-image or historical-schema compatibility test. Restart covered control process/database access, not active-job or runner recovery. Docs and runner images were not built.

Local OIDC issuer and CA only. Customer IdP/TLS, KMS, hosted topology/isolation, runner recovery, published-image upgrade, external providers and paid APIs remain uncertified. Docker reported no swap-limit support; this is not isolation or load qualification. No customer repository or existing demo service changed.

## Install packaging gaps observed

- Install guide omits creating migration/runtime roles and applying scripts/runtime-grants.sql. Compose Postgres creates only configured POSTGRES_USER; it does not create reforge_migrator or reforge_runtime.
- .env.example names migration role reforge_owner, while local-development docs and grants use reforge_migrator.
- Compose interpolation requires REFORGE_RUNNER_DIR with runner profile disabled. Harness supplies inert run-local path; no runner starts.
- .env.example leaves required POSTGRES_PASSWORD empty and uses HTTP public URL, which production configuration rejects without development mode. Document mandatory values and TLS termination/public origin.

Harness supplies database roles/grants and runner path only in disposable environment. No Compose or install-guide edits made here.
