# T26 Compose bootstrap review

Status: Compose clean-install bootstrap and local production install checks pass. T26 remains partial pending external certification.

Compose startup now creates or reconciles `reforge_migrator` and `reforge_runtime` before migration, applies `scripts/runtime-grants.sql` after migration, then starts the control plane. Role passwords are taken from explicit settings or their credential-bearing database URLs. Runtime and migration roles are forced to non-superuser login accounts without database/role creation, replication or RLS bypass privileges.

Before any role or ownership changes, bootstrap rejects empty credentials, a database owner other than `POSTGRES_USER` or `reforge_migrator`, and role membership for either Reforge account. It never drops database objects or revokes role memberships. Custom-owned databases need an operator-reviewed ownership change before Compose startup. Runner profile interpolation now works when `REFORGE_RUNNER_DIR` is unset.

The install guide covers required secrets, HTTPS termination, OIDC callback URI, docs proxy routing, first-organisation bootstrap values, and safe existing-volume upgrades.

| Check | Result | Evidence |
|---|---|---|
| Compose config without runner directory | Pass | `.local/t26-compose/config-verified.log` |
| Fresh PostgreSQL, role bootstrap, migration, grants | Pass; migration owner owns DB; runtime can read application tables, cannot read migration ledger or delete audit rows; 33 migrations | `.local/t26-compose/compose-verified.log`, `state-verified.log` |
| URL-only role credentials | Pass with direct role password variables omitted; roles created, migration authenticated and grant service connected through the migration URL | `.local/t26-compose/compose-url-only.log`, `state-url-only.log` |
| Existing-volume repeat | Pass; migration history and sentinel row preserved; both roles remain password-authenticated, non-superuser and unable to create DBs/roles | `.local/t26-compose/repeat-final-latest-compose.log`, `repeat-final-latest-state.log` |
| Empty role credentials | Refused before role creation or owner transfer | `.local/t26-compose/empty-password-refusal.log`, `empty-password-state.log` |
| Custom database owner | Refused with nonzero exit; owner and sentinel row preserved | `.local/t26-compose/custom-owner-refusal-verified.log`, `state-existing-verified.log` |
| Inherited role membership | Refused with nonzero exit; membership remained unchanged | `.local/t26-compose/membership-refusal-verified.log`, `membership-state-verified.log` |
| Strict documentation image build | Pass | `.local/t26-compose/docs-build-final.log` |
| Container install, failed-migration retry, fixture-off production startup, local TLS/OIDC, restart and encrypted restore | Pass | `.local/t26-compose/container-install-check-final.log`; `.local/t28a-install/t28a_20260923040631_2661/` |

Local fixture checks do not certify customer OIDC/TLS, KMS, hosted isolation, runner recovery, published-image upgrade or external providers. T26/T28 release acceptance remains open for those environments and scenarios.
