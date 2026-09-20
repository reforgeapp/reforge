# Implementation progress

Updated: 2026-09-20. Coordinator: Astra. Persistent goal active; full T01–T28 scope authorised. No live provider mutation, paid API qualification, publication or external deployment authorised.

## Execution state

- Planning-only directory inspected; no pre-existing Git repository or product code. Original specifications preserved.
- Read PLAN, product, GUI, architecture, contracts, policies, decisions, backlog, agents and validation specifications.
- T01 contract freeze reviewed and locally verified. Coordinator retains shared contracts, manifests, generated files and migration numbering. T02/T04 ready.
- Follow actual dependency edges; local completion unlocks implementation, external certification separately gates release.
- All commits use real current metadata. No README edits or source comments.

## Tickets

| Ticket | Status | Owner | Files / dependencies | Evidence / remaining |
| --- | --- | --- | --- | --- |
| T01 | local complete | Astra | foundation, shared contracts; none | Build/check/unit tests; PG18.6 race/isolation/migration tests; independent Astra review fixed |
| T02 | local complete | Astra worker t02_identity | auth/store; T01 | OIDC/scopes/bootstrap implemented; signed local OIDC + PG race tests pass; root all-package race/vet/build passed |
| T03 | not started | Astra | connections/secrets; T01 T02 | Envelope encryption, network routes |
| T04 | in progress | Luna worker t04_shell | web shell; T01 | Browser and cache isolation |
| T05 | not started | Astra | workflow; T01 T02 T03 | Leases, budgets, outbox, SSE |
| T06 | not started | Astra | policy; T01 T02 | Deterministic corpus |
| T07 | not started | Astra | runner; T03 T05 T06 | Sandbox and private routes |
| T08 | not started | Luna/Astra | forge/github; T01 T03 | Real adapter, guarded mutations |
| T09 | not started | Luna/Astra | forge/gitlab; T01 T03 | Real adapter, guarded mutations |
| T10 | not started | Astra/Luna | forge/gitea; T01 T03 | Disposable real server certification |
| T11 | not started | Luna/Astra | inventory; T05 T08 T09 T10 | Async reconciliation |
| T12 | not started | Luna | model/openai; T01 T03 T05 | Streaming contracts and live gap |
| T13 | not started | Luna | model/anthropic; T01 T03 T05 | Streaming contracts and live gap |
| T14 | not started | Luna | model/google; T01 T03 T05 | Continuation contracts and live gap |
| T15 | not started | Luna | model/compatible; T01 T03 T05 | Real local inference qualification |
| T16 | not started | Astra | agent; T03 T05 T06 T07 | Documented permitted runtime routes |
| T17 | not started | Luna | portfolio GUI; T02 T03 T04 T11 T12–T16 | Backend-connected onboarding |
| T18 | not started | Astra/Luna | discovery; T05 T06 T11 | Bot ownership and deduplication |
| T19 | not started | Astra/Luna | repair/recipes; T07 T12 T13 T14 T15 T18 | Real repair loop and trusted validation |
| T20 | not started | Luna | evidence GUI; T04 T17 T18 T19 | Connected finding/run/change flows |
| T21 | not started | Astra | merge; T06 T08 T09 T10 T19 | Exact revisions/native protection |
| T22 | not started | Astra | pipelines; T05 T06 T08 T09 T10 T21 | Native approval and correlation |
| T23 | not started | Astra | GitOps; T06 T08 T09 T10 T21 | Protected delivery and provenance |
| T24 | not started | Luna | control GUI; T04 T06 T20 T21 T22 T23 | Policy/deployment/usage/audit |
| T25 | not started | Astra/Luna | campaigns; T05 T06 T18 T19 T21 T23 T24 | Pinned canaries/fairness |
| T26 | not started | Luna/Astra | deploy/operator; T02 T03 T07 T16 T22 T23 | Install, restore, operations |
| T27 | not started | Astra | qualification; T19 T21 T22 T23 T25 T26 | Isolation/concurrency/recovery |
| T28 | not started | Astra/Luna | handoff; all prior | Browser/load/corpus/licences; G5 open |

## Gates

G0 foundation and identity locally verified; subsequent artifact/SSE isolation rechecked at their tickets. G1–G5 pending. No external integration or release certification claimed.

## Environment and external requirements

- Host has Go 1.23.5, Node 26.7.0, npm 11.19.0, PostgreSQL 12 binaries and cached Chromium. Will install supported build dependencies in task-local paths.
- Docker client exists; daemon unavailable even outside sandbox. PostgreSQL 18.6 successfully built from checksum-verified source in `/tmp/reforge-postgres`; live loopback server port 55432, disposable `reforge_dev`/`reforge_test` databases. Gitea and sandbox alternatives remain to investigate.
- GitHub/GitLab dedicated test credentials, paid model test budgets, account/topology entitlement evidence and hosted isolation infrastructure not supplied. Continue all independent local implementation; record precise certification actions per capability.

## Resume

Read this file, `agents.md`, `backlog.md`, current Git diff and active worker ownership. Continue T02/T04; T03 after T02. Foundation server currently running in tool session 57124 on port 8080; stop/rebuild when integrating auth. Environment is in ignored `.local/development.env`. Never interpret this checkpoint as completion of the full request.

## T01 evidence — 2026-09-20

- `make build`, `make check`, `make test`: passed. Go 1.27.1; React 19.3.0; PostgreSQL 18.6. Generated SQL/OpenAPI outputs reproduce, and missing-file drift regression correctly fails.
- Real PostgreSQL `make test-integration` with race detector: empty migration, idempotent migration, failed migration rollback, applied checksum rejection, 50 competing tenant transactions, no pooled context leakage, denied unscoped insertion, rejected schema-owner/inherited-owner credentials. All passed.
- Running built Go/Gin server: `/healthz` and `/readyz` 200; `/api/v1/meta` explicitly labels development/fixture auth; SPA deep links serve built assets.
- Independent Astra review completed; fixes cover inherited privileges, credential serialization/logging, fixture label, generation manifests, dependency notices and all test database URL guards. No live provider calls.
- Apache-2.0 text, dependency inventory and collected notices added. Final distribution inventory reruns at T28.

## Active ownership

- `t02_identity` (Astra): `internal/auth/**`, `internal/httpapi/identity.go`, `test/integration/auth_test.go`, `docs/implementation/t02-identity.md`. SQL proposal in auth/schema.sql; coordinator alone assigns migration002 and wires startup/OpenAPI/manifests.
- `t04_shell` (Luna): `web/src/main.tsx`, `web/src/app/**`, `web/src/components/**`, `web/src/styles/**`, `web/src/api/client.ts`, `web/tests/shell.spec.ts`, `web/playwright.config.ts`, `docs/implementation/t04-shell.md`. No manifests/generated types.
- Coordinator: shared interfaces/manifests, migrations, startup wiring, API contract, integration/review and progress. No other active workers.

## Local qualification preparation

- Signed Gitea 1.27.3 binary verified against documented release key fingerprint `7C9E68152594688862D62AF62D9AE806EC1592E2`; `/tmp/reforge-gitea/gitea`. No provider contract scenarios run yet.
- gVisor `release-20260914.0` checksum verified; binaries and sidecars under `/tmp/reforge-gvisor/bin`. Trusted OCI probe succeeded using `runsc --rootless --network=none --platform=systrap --ignore-cgroups run`; host home/socket inaccessible and loopback PostgreSQL unreachable. Parent-created namespace variant failed creating gofer; built-in rootless variant works. This is feasibility evidence only: resource limits, hostile corpus and hosted isolation still require T07/T27 proof.

## T02 review evidence

- Astra implementation and independent Astra review complete. Reviewed actual source plus SQL and tests. Fixed inherited grant persistence, sibling-domain OIDC cookie injection and mutation/revocation races.
- Worker `go vet` and expanded signed-local-OIDC/PG18.6 `-race` tests pass: PKCE/state/nonce/browser binding, restart-persistent login state, bootstrap one-use/expiry, cross-tenant/team/child scopes, CSRF/Host, logout, saved-session revocation, last-owner concurrency and queued mutation vs revocation. Root `go test -race ./...`, `go vet ./...` and server build all passed against PostgreSQL18.6.
- Identity production setup uses configured OIDC and authenticated one-use bootstrap; no local password/token-harvesting route. `WithMutation` orders org/session locks before current authority; later mutation services must use it.
- SSE/artifact dynamic scope tests remain for T05/T07/T27. Retention and rate limiting remain T26/T27 operational integration.
