# T24 controls

Policies, Usage and Audit use authenticated Go APIs and current repository permissions. Deployment and GitOps controls are documented in the T22/T23 reports.

- Policies: choose repository and organisation/team/repository scope; inspect inherited layers and effective rules; save an immutable version; simulate; activate its exact hash with the current binding version. Repository policies can select a bound primary team. Edited drafts require another save. Restrictive policies remain activatable even when their simulation blocks maintenance. Native provider approvals remain separate.
- Usage: apply repository/team/recipe/provider/connection/state/time filters; distinguish recorded tokens, estimated API cost, subscription quota and maximum holds for unknown outcomes. Inspect scoped budgets and route qualification. Owners configure caps and pause; budget periods are immutable after creation. Missing or conflicting budgets remain actionable errors. Route qualification is read-only.
- Audit: filter actor/action/repository/time; inspect escaped event data; export each selected loaded page as NDJSON. Each export remains bounded to 100 events and current access. Organisation events require owner/admin; other actors see authorized repository events. Earlier pages require separate exports.

API additions: `GET /usage`, `/usage/summary`, `/audit-events`, `/audit-events/export` under `/api/v1/orgs/{orgID}`. Dates require RFC3339; upper bounds are exclusive. Pagination uses timestamp/UUID tuples. Exports return `X-Next-Cursor` and `X-Export-Complete`.

Local evidence: PostgreSQL/API race suite1.127s (`.local/insights-pg-root3.log`), generated clients, Go vet and production frontend build passed. Seven browser checks passed4.3s (`.local/insights-browser-final.log`), including actual policy activation, persisted budget reload and downloaded audit evidence at390px. Fixture scenarios separately cover invalid filters, stale scope drafts, role restrictions, exact export cursors and primary-team routing. Live browser fixtures require explicit local development opt-in and create isolated acceptance organisations; no forge/model calls occur.

T24 does not certify external providers, subscription entitlement, hosted isolation or G5. Remaining onboarding qualification and distribution work stay tracked separately.
