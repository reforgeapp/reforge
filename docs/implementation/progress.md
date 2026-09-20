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
| T03 | local complete | Astra coordinator + network worker | connections/secrets; T01 T02 | Operator/KMS envelopes, write-only credentials, revoke/rotate/rewrap, guarded routes; race/check/build passed |
| T04 | local complete | Luna worker t04_shell | web shell; T01 | 8 browser scenarios, keyboard/390px/axe/deep links/cache revocation; root reviewed |
| T05 | local complete | Astra coordinator/workers | workflow; T01 T02 T03 | PG race/recovery/fairness/budget contention; HTTP/SSE policy+scope+revocation; generation/vet/build pass |
| T06 | local complete | Astra worker t02_review | policy; T01 T02 | Corpus + PG immutable activation/scopes + HTTP simulation/CSRF/version checks passed |
| T07 | in progress | Astra | runner; T03 T05 T06 | Sandbox and private routes |
| T08 | in progress | Luna worker t04_shell | forge/github; T01 T03 | Real adapter, guarded mutations |
| T09 | in progress | Luna/Astra | forge/gitlab; T01 T03 | Real adapter, guarded mutations |
| T10 | in progress | Astra coordinator | forge/gitea; T01 T03 | Disposable real server certification |
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

Read this file, `agents.md`, `backlog.md`, current Git diff and active worker ownership. T01–T04/T06 locally complete. Continue T05 lease/budget review+HTTP/startup, T08 GitHub worker, T10 Gitea. Latest server runs8080 in tool session88512; rebuild/restart when wiring T05. Environment is in ignored `.local/development.env`. Never interpret this checkpoint as completion of the full request.

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

## T03 active ownership

- Coordinator: `internal/secrets/**`, `internal/connections/**`, `internal/httpapi/connections.go`, migration003, API/startup/config integration.
- Astra `t02_identity` reused for T03 network child: `internal/network/**`, `docs/implementation/t03-network.md` only. Fixed-origin, DNS-pinned transport; private destinations require matching enrolled runner and explicit CIDRs.
- Luna `t04_shell` continues assigned shell ownership; root review requests cover protected deep links, org mismatch, route-managed query state, request cancellation and identity cache clearing.

## T03 / T04 checkpoint

- T03 encrypted persistence/rotation/restart/tenant isolation/revocation race tests pass against local PG18.6. Probe executes under current org/session authority locks with a 10-second context. Raw provider errors discarded; secrets excluded from JSON/log formatters. Network transport race tests pass. Hosted KMS backend is being added to meet architecture requirements; T03 remains open until reviewed.
- T04 initial browser suite: four passing tests against live Go server, including sign-in, search, dialog and axe. Final review adds identity cache invalidation, back navigation and narrow viewport checks; business-route implementation remains T17/T20/T24.
- T06 deterministic policy engine and version storage in review; migration004 applied dev/test. Worker owns internal/policy and t06-policy documentation. Root wires HTTP/contract later.
- Current worker ownership: t02_identity owns internal/secrets and t03-kms documentation; t02_review owns internal/policy and t06-policy documentation; t04_shell retains web shell files and t04 evidence. Coordinator owns connections/API/config/manifests/migrations/progress.

## T03 completion evidence

- Integrated `go test -race ./...`, `make check`, generation drift and application build passed. Initial generated UUID dependency mismatch was corrected with the existing string ID mapping; final whole-package run passed.
- AWS SDK config1.33.5/KMS1.61.0 use standard credentials, explicit immutable key ARNs and tenant/connection/version context. Contract tests cover wrong context/key, rotation, outage, cancellation and no endpoint override. No live AWS call or certification claimed.
- Approved private routes require real enrolled-runner registration (T07); unavailable provider adapters stay disabled pending their owning tickets.

## Active ownership after T03

- Astra t02_identity: T05.1 `internal/workflow/**`, `test/integration/workflow_test.go`, `t05-workflow.md`; durable leases, fences, pauses and event replay.
- Astra t02_review: T05.2 `internal/budget/**`, `t05-budget.md`; hierarchical reservation/accounting.
- Luna t04_shell: T08.1 `internal/forge/github/adapter.go`, `adapter_test.go`, `t08-github.md`; inventory/events/PR CRUD only. Root later handles App authentication, branch guards/protection/merge/delivery.
- Coordinator: policy HTTP integration complete, all shared files/migrations/API, next forge security and Gitea local integration. T05 migrations005/006 installed;007 corrects custom-only period validation without rewriting applied checksums.

## T04 / T06 integration evidence

- T04 root reran eight browser scenarios: all pass. Same-page revocation fixture proves cache purge without reload; real backend identity revocation remains covered by T02 PG tests and later SSE/artifact qualification. Identity changes hide scoped content until cache clearing completes.
- T06 corpus/real PG tests and HTTP create→simulate→activate/history/stale-version/CSRF checks pass under race detector. Integrated vet/build pass. `REFORGE_POLICY_FILE` optionally supplies trusted deployment constraints; absent organisation policy always disables automation. Version history and effective policy endpoints are connected.

## Current integration checkpoint

- Commits:20eb5e0 T03,8093e69 finalT04fixes,04a0fac T06. T05/T08work in progress, do not stop.
- T05 root owns `internal/httpapi/workflow.go` (task/pause/events/replay/SSE) and `test/integration/events_http_test.go`. Actual HTTP streaming scope/session revocation test passed under race detector (2.089s). Startup/OpenAPI/budgetroutes/policy-admission still to wire.
- T05 review fixed audit actor attribution, JSONinteger preservation, bounded recovery, registered scope validation, permanent pause fence revocation, injected freshpolicy/dependency check for budgetdispatch, returnedspend truth and immutable modelroute. Migration008 adds model_route; applied dev/test. Root needs final integrated tests and review before localcomplete.
- Gitea1.27.3 running loopback53000 session33422, work `.local/gitea`. Three local qualification accounts (admin,bot,reviewer), password/token files0600 ignored. Bot/reviewer tokens scoped repository/issue/readuser/org. Swagger saved `.local/gitea-api.json`. No customerrepo or paidprovider touched.
- Gitea API supports PUT branches/{branch} with old_commit_id,new_commit_id,force=false. Plan staged immutable app commit then exact-old guard; must qualify actual head movement, outdated reviews, requiredchecks and stricttarget before claims. No Giteaadapter code yet.
- Forge Change contract adds head_repository and target_repository immutable RepoRefs so fork source identity is not inferred from names. Adapters must populate and controllers bindboth.

## Active checkpoint — T05 integration / T07–T10

- Root integrated HTTP workflow/task/pause/replay/SSE and budget configuration/reservation reads. `control.Authority` resolves current policy and explicit model+connection/route admission; privileged action validators remain fail-closed until their owning controllers register authoritative evidence checks. No synthetic evidence is supplied at queue admission.
- Real PG `TestTaskBudgetHTTPAndCurrentPolicyDispatch -race` passed: missing policy/unapproved route blocked, versioned budget writes, fresh policy rejects reserved dispatch, unregistered publication denied, CSRF required. OpenAPI schemas/routes added; generation and broader checks next.
- Final T05 worker suites passed: workflow six PG race tests including 100 claims and 201 expired writers; budget five PG race scenarios including 100 contenders/tightest ceiling. Root still reviews integrated diff before marking local complete.
- Disposable local DB passwords rotated after accidental worker tool-output exposure; source current ignored `.local/development.env`. Existing Go server needs restart with new credentials.
- T07 Astra `t02_identity`: owns internal/runner/**, artifact/store.go+tests, httpapi/runner.go, t07 doc, and workflow/leases.go additions for server-scoped claims, same-transaction completion and permanent revocation. Coordinator owns sandbox runtime/cmd/config/startup/migrations/API. Migration009 applied dev/test: pools, enrollments, runner/job credentials, artifact metadata and composite task/repo/attempt FKs.
- T09 Luna `t04_shell`: owns forge/gitlab/adapter.go, adapter_test.go and t09 doc; inventory/events/MR CRUD, excluding guarded branches/protection/merge/delivery.
- T10 Astra `t02_review`: owns forge/gitea/**, test/forge/gitea/** and t10 doc; full adapter plus actual local contract scenarios. Local read-only inspector token available .local/gitea/reforge-inspector.token. Gitea requires repo-admin role for rule inspection, so read-only inspector is separate from nonadmin operational bot; default rules unknown without inspector. All mutations remain bot-sourced. Root to add explicit tenant-bound inspector connection config/factory.
- GitHub Luna slice delivered, root review found remaining issues before integration: installation inventory response is an object (currently array decoder), operation lookup must itself check actual connection actor before CreateChange adopts it, no empty-file rejection, bounded pagination loop must not trust repeated next pages, repository identity should derive from base.repo when response lacks top-level repository, current target ref must be fetched for authoritative ReadChange. Root will fix with T08 App/security work. No GitHub certification claimed.

## T05 local completion

- Commits ba27cd0 durable fenced workflow and e3a2f97 hierarchical reservations. Root read worker source and reviewed scope/policy/pause/idempotency/recovery fixes before integration.
- Final integrated PG race run: budget2.471s, policy1.132s, integration5.716s, all passed. Targeted vet, SQL/OpenAPI generation drift and server build passed. A broad make check encountered formatting drift in concurrently written T07 files; this was not a test failure, final whole-workspace check remains T07 integration/T28.
- Policy activation and budget configuration/debt emit durable scoped events in the same transaction as audit/accounting. Root tightened scoped-owner repository/team finance access. HTTP task/budget/state routes, actual SSE scope+revocation and startup are wired. Deferred native controllers must register authoritative action validators; unknown actions remain denied.
- Migration009 belongs to T07 and remains uncommitted with runner work. Generated SQL runner types remain unstaged; preserve them during later generation.
- Gitea real tests found ordinary merge stale-target race despite outdated-branch protection. Fast-forward-only rejected that race. Native status contexts do not bind publisher: spoofed same-name success could override trusted failure. T10 certifies only demonstrated method/guarantees; policy requiring absent enforcement stays blocked.
