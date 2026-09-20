# Implementation progress

Updated: 2026-09-21. Planning-only revision. Existing implementation evidence below is historical and preserved; no implementation restart, worker launch, build, certification or external mutation authorised by this revision. Current progress source authoritative for status.

## Planning revision status

- Prior GUI completion acceptance superseded by T29 full GUI rebuild/refactor. Prototype historical; no visual authority.
- T29 full GUI rebuild, T30 MkDocs documentation and T31 custom command runtime are new planned tickets. T28 dependency graph explicitly names every prerequisite and remains acyclic.
- Current allocation override: root/Astra architecture and final review; Luna bounded implementations including sensitive slices under Astra review; no implementation workers during current pause; maximum three Luna workers when resumed.
- Planning checks: 31-ticket dependency graph acyclic; local Markdown link targets and fence balance pass across changed/new docs. No application tests run or required for this docs-only revision.

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
| T08 | local complete | Astra worker t02_identity | forge/github; T01 T03 | Real adapter, guarded mutations |
| T09 | local complete | Luna/Astra | forge/gitlab; T01 T03 | Real adapter, guarded mutations |
| T10 | local complete | Astra coordinator | forge/gitea; T01 T03 | Disposable real server certification |
| T11 | local complete | Luna/Astra | inventory; T05 T08 T09 T10 | Async reconciliation |
| T12 | local complete | Luna | model/openai; T01 T03 T05 | Streaming contracts and live gap |
| T13 | local complete | Luna | model/anthropic; T01 T03 T05 | Streaming contracts and live gap |
| T14 | local complete | Luna | model/google; T01 T03 T05 | Continuation contracts and live gap |
| T15 | local complete | Luna | model/compatible; T01 T03 T05 | Real local inference qualification |
| T16 | in progress | Astra | agent; T03 T05 T06 T07 | Documented permitted runtime routes |
| T17 | in progress | Luna | portfolio GUI; T02 T03 T04 T11 T12–T16 | Backend-connected onboarding |
| T18 | local complete | Astra/Luna | discovery; T05 T06 T11 | Bot ownership and deduplication |
| T19 | local complete | Astra/Luna | repair/recipes; T07 T12 T13 T14 T15 T18 | Real repair loop and trusted validation |
| T20 | local complete | Luna | evidence GUI; T04 T17 T18 T19 | Connected finding/run/change flows |
| T21 | local complete | Astra | merge; T06 T08 T09 T10 T19 | Exact revisions/native protection |
| T22 | local complete | Astra | pipelines; T05 T06 T08 T09 T10 T21 | Native approval and correlation |
| T23 | local complete | Astra | GitOps; T06 T08 T09 T10 T21 | Protected delivery and provenance |
| T24 | local complete | Luna | control GUI; T04 T06 T20 T21 T22 T23 | Policy/deployment/usage/audit |
| T25 | in progress | Astra/Luna | campaigns; T05 T06 T18 T19 T21 T23 T24 | Pinned canaries/fairness |
| T26 | not started | Luna/Astra | deploy/operator; T02 T03 T07 T16 T22 T23 T31 | Install, restore, operations |
| T27 | not started | Astra/Luna review | qualification; T19 T21 T22 T23 T25 T26 T29 T30 T31 | Isolation/concurrency/recovery |
| T28 | not started | Astra/Luna | handoff; explicit T01–T27,T29–T31 | Browser/load/corpus/licences; G5 open |
| T29 | not started | Luna/Astra review | GUI rebuild; T04,T16,T17,T20,T24,T25,T31 | Route coverage, visual regression and SaaS/OSS workflow evidence pending |
| T30 | not started | Luna/Astra review | MkDocs; T16,T19,T21,T22,T23,T26,T29,T31 | Container docs build/serve and linked help pending |
| T31 | not started | Luna/Astra review | custom runtime; T03,T05,T06,T07 | Profile protocol, qualification and container acceptance pending |

## Gates

G0 foundation and identity locally verified; subsequent artifact/SSE isolation rechecked at their tickets. G1–G5 pending. No external integration or release certification claimed.

## Environment and external requirements

- Host has Go 1.23.5, Node 26.7.0, npm 11.19.0, PostgreSQL 12 binaries and cached Chromium. Will install supported build dependencies in task-local paths.
- Docker client exists; daemon unavailable even outside sandbox. PostgreSQL 18.6 successfully built from checksum-verified source in `/tmp/reforge-postgres`; live loopback server port 55432, disposable `reforge_dev`/`reforge_test` databases. Gitea and sandbox alternatives remain to investigate.
- GitHub/GitLab dedicated test credentials, paid model test budgets, account/topology entitlement evidence and hosted isolation infrastructure not supplied. Continue all independent local implementation; record precise certification actions per capability.

## Resume

Root sole Astra X-High; max three Luna workers. T22/T23 locally complete; T24 locally complete; T25 architecture/core next. Current server2619 on8080; secrets in ignored `.local/development.env`. Root owns controller/API/migrations; All Luna workers idle after T24 review; root owns shared integration. Latest checkpoints below contain evidence. T16 production custody/wiring, T17 qualification GUI and T24–T28 remain. G5 open. User resumed work despite persistent-goal UI pause; no tool resume operation.

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

## Current checkpoint — runner and provider integration

- T10 root read and reviewed full adapter, then fixed operation target binding, case-sensitive protection, current authorization before every branch write, invalid review identities, empty required responses and uncertain create reconciliation. Real local Gitea1.27.3 race suite passed: package1.098s / native scenarios28.612s; final focused regressions passed. See t10-gitea.md for certified local guarantees and known native enforcement limits. T22/T23 delivery remains separate, G5 not passed.
- T07.1 root reviewed controller/artifact source and requested issuer reauthorization, invalidation after pool updates, and scope-filtered pagination. Eleven PG runner race tests pass2.489s; artifact1.046s, affected workflow2.514s. Startup/API wiring now added; actual outbound private dispatcher and runner command still pending. Retention service exists; periodic tenant scheduling will wire with T11/T26.
- T07.2 Astra t02_review owns internal/sandbox/** except contracts.go, cmd/sandbox-tool/**, test/sandboxprobe/** and t07-sandbox.md. Rootless managed run fixed create incompatibility; actual gVisor boundary test passes. Additional process-termination/restart/asset-integrity checks ongoing. Host cgroup filesystem read-only; production resource enforcement correctly remains unavailable here.
- T08 Astra t02_identity now owns internal/forge/github/** + t08-github.md for App authentication, native CAS, protection/rulesets/merge-queue safety and basic-slice review fixes.
- T13 Luna t04_shell owns internal/model/anthropic/** + t13-anthropic.md. Official SDK1.74.0 installed; Google SDK1.71.0 pinned for T14. No paid API calls.
- Root owns all shared manifests/contracts/migrations/OpenAPI/startup/progress, runner command, private transport and T12 review fixes. OpenAI review found invalid continuation shape/order and insufficient schema/usage bounds; fixing before local completion.

## Reviewed integration evidence

- T07 controller/root integration regression: runner2.354s and HTTP control1.574s under race detector passed against disposable PG. Artifact package was filtered out in that command; its full worker suite1.046s was separately run. Generation and targeted vet passed.
- Root rebuilt static sandbox helper/probe and independently reran the real gVisor suite:8.298s passed. Reviewed all runtime/guest/process source. Fixed Close attempting every workspace on cleanup failure while retaining lock, and opened-file verification before truncation; lifecycle race regressions pass. T07 controller/runtime are ready; private outbound connector and command integration remain.
- T10 committed e22c355. T12 root corrected continuation history/order, encrypted reasoning preservation, strict output/request/tool bounds, byte estimates, usage-field validation and post-dispatch uncertainty; shared schema and OpenAI race suites passed1.029s/1.033s before final doc changes. Live paid certification remains absent.
- T07.3 Astra t02_review now owns internal/privateconnector/**, internal/runner/private.go, internal/httpapi/privateconnector.go and t07-private.md. Ready-first fixed-operation dispatch avoids waiting for runner polls while holding tenant mutation locks; credentials/grants remain ephemeral, durable outbox/budget dispatch is committed first and lost responses remain uncertain. Root owns startup/command/factory integration.

## Current model / runner client checkpoint

- Commits79b32dd runner/artifact API and8f4ecb9 sandbox. OpenAI+shared schema committed e358967/5b179a0; root reviewed and meaningful race tests passed. This does not qualify live model APIs.
- T13 root read all SDK adapter/tests. Fixed incomplete history, response-body leak, complete model pagination, metadata/usage field presence, cache-write accounting, empty-argument tools and callback buffering. Real SDK HTTP fixture round-trip retains thinking signature, whole history and closes both streams. Final race1.050s and vet passed. OfficialAnthropicSDK1.74.0.
- Luna t04_shell now owns internal/model/google/** + t14-google.md. OfficialGoGenAI1.71.0; root specified total billable thought/output accounting and full opaque continuation.
- Root internal/runnerclient implements credential persistence/restart/rotation, scoped claim/progress/artifact/broker/result protocol, cancellation heartbeats and redirect denial. Local HTTP race tests passed1.032s. cmd/runner and full processor integration remain pendingT07/T19; do not represent this as the completed runner application.
- T16 source/installed-runtime research begun independently: CodexCLI0.150.1 exists, official app-server JSON schemas generated into /tmp/reforge-codex-schema using read-only tooling. No login/logout, model turn or paid request performed. Implementation awaits T07 integration; entitlement/isolation must be qualified before activation.

## Current checkpoint — reviewed adapters and private onboarding

- T08 reviewed full source/tests and committed ae9c8f7. Root corrected authenticated probe, inventory identity, request/response boundaries and408 uncertainty. Final race1.161s/vet pass. Live Cloud/GHES native enforcement certification remains open; final retarget-race review noted for follow-up.
- T07.3 full source reviewed; worker real PG/Gitea suite14.050s passed, including clean-environment supervisor process, lock/revocation,6MiBresult transport and saturated20-connection pool. Ready refresh precedes authorization TX, exact runner/hash is locked in that TX, no DB acquisition during delivery/result. Replay tombstones bounded5min/10k, tested10,001operations. Dedicated/sticky controller topology explicit.
- Root integrated actual adapter factories, private connection testing and cmd/runner enroll/connector. Private ResolveTx returns noHTTPclient and requires exact active runner; publicfactoryrejectsprivateRoute. BrowserHTTP→encryptedvault→enrolledsupervisor→realGitea1.27.3 probe/revocation regression passed1.496s with other connection tests. Empty onboarding pools carry no job scope and require organisation-wide owner for enrollment. Fullrepairprocessor remainsT19.
- T14 Google root review found unsafe non-STOP tool delivery, native part reordering, missing legacy call IDs, absent usage treatedaszero, SDK float64 roundtrip and callback uncertainty. Astra t02_review owns Google fixes; concurrent/precision/signature/oversize regression reports1.870s passing, final handoff pending.
- T15 Luna basic adapter delivered. Root review fixes bounded/duplicateindices, terminalreason, pinnedmodelcaps, opaque numeric preservation and full new-input continuation. Root race1.032s passed before expanded regressions. ActualOllama0.34.2 officialrelease SHA256verified in /tmp/reforge-ollama; disposableCPUserver setup underway, no paidAPI.
- Current workers: Astra t02_identity GitLab fullsecurity slice; Astra t02_review Googlereviewfix; Luna t04_shell T17.1connections/runnersrealGUI child. ParentT17stillopenuntilinventory/modelagentcapabilities integrated. Root ownssharedcontracts/startup/APIs/factories/T11architecture/runnercommand.


## Current checkpoint — inventory foundation and real compatible inference

- T09 full GitLab slice reviewed and committed 27ced85. Root race/vet pass (cached final rerun); fixtures cover retargeting, exact source, native trains, incomplete approval/protection and publisher identity. Live GitLab.com/Self-Managed version/tier certification remains open.
- T14 Google fixes reviewed and committed cbbb1a1; final root race1.884s/vet passed. No paid API calls. T15 compatible adapter reviewed and committed a80a011; final unit race1.034s/vet passed. Real local Ollama0.34.2/Qwen3:0.6b completed tool+continuation on both Chat Completions and Responses in11.192s. vLLM and paid provider qualification remain open.
- T07.3 committed0e32af6; final real private PG/Gitea race14.178s, runner2.518s passed. Root factory/CLI startup and generic private forge/model metadata extension currently uncommitted. Model inference through enrolled transport still awaits durable budget/controller integration; metadata tests do not imply inference qualification.
- T11 Astra t02_identity owns internal/inventory, inventory HTTP, integration scenarios and t11 doc. Reviewed schema copied to migration010 and applied dev/test. Global scheduler catalog contains tenant UUID only; all operational data stays tenant RLS. Root delivers providers.Read fresh-authorization gateway and pure verified webhook decoder. Partial scans cannot remove access; imports remain asynchronous.
- T16 Astra t02_review owns official app-server bridge/qualification matrix. Actual0.150.1 version/schema inspected; no account cache/login/logout/paid turn accessed. Fixture tests underway; activation requires dated entitlement, identity custody, isolated topology and quota evidence.
- T17.1 Luna delivered GUI; root review found invalid auth/billing defaults, unusable private initial creation, stale selected versions and incomplete functional browser checks. Worker correcting these; parentT17 remains in progress. Server restarted latest backend at8080. T18 design reconnaissance assigned after those corrections; no dependent implementation yet.
- All later tickets T18–T28 remain required. G5 is not passed. Persistent goal remains active; no budget invented.

- Follow-up evidence: 39472eb rejects GitHub direct/queue retargeting; final race1.092s. Reviewed public/private provider gateway integration passed3.325s with real PG/Gitea/Ollama metadata and denied-callback test. SQL/OpenAPI generation passed using pinned cached generators. Model inference still has no private paid-turn handler until T19 durable accounting is integrated.


## Current checkpoint — cost controls and T11 integration

- User resumed implementation with Astra coordinator only; no Astra workers. Fresh bounded Luna workers own connection GUI, inventory GUI and pure dependency detectors. All handoffs use caveman-full; coordinator retains sensitive implementation and review. Earlier worker ownership entries above are historical.
- T11 source/tests reviewed completely. Fixed stale change-cache presentation after access removal and verified explicit resync restores polling after terminal credential failure. Combined root real-PG inventory and private Gitea/Ollama integration race suite passed8.811s; targeted vet passed. Includes 1,000-repository asynchronous import, 100 competing claims, durable scheduler restart, signed webhook replay/rotation and enrolled private HTTP sync/import/refresh.
- T16 bridge committed a912d2b after root review and close-during-startup regression; actual installed Codex0.150.1 schema check plus race2.647s passed. No account login/cache/paid turn accessed. Production qualification/factory remains outstanding.
- T07.4 pinned source acquisition reviewed; native Gitea pagination/branch-movement proof passed worker7.287s. Fixed-operation gateway integration and coordinator acceptance pending. GitHub native root hash; GitLab/Gitea explicitly record immutable-ref API proof, never an invented native root.
- T17 GUI remains in progress; T18 detector child underway, coordinator owns discovery persistence/cooperation. T19–T28 remain required; G5 not passed. Goal UI currently reports paused despite resume instruction; tools expose no resume operation. Work continues under user authorization.

## Current checkpoint — discovery and GUI review

- Root only Astra; new/continued workers Luna, caveman-full, bounded ownership. Currently luna_connections owns connection GUI webhook followup; remaining workers idle. Root owns discovery/core security and integration.
- Commits4487848 inventory,002bf4d route-registration fix,a9ba270 verified pinned source reads. T11 local complete; T08/T09/T12–T15 local adapters complete, external qualification separate.
- T17 real private Gitea browser journey passed7.7s: enrollment, credential connection/probe, native repository preview/select/import/detail freshness, rotation and revocation. Inventory keyboard/narrow/error/back/deep-link/saved-view tests worker3pass. Root fixed duplicate modal IDs, detail navigation clearing rows and saved-view error overwrite. Webhook GUI and baseline remain.
- T18 migration013 applied to disposable dev/test; durable scoped findings/config/scans/history, versioned suppression, immutable bot identities, overlapping repair guards, source/check discovery, import advisories and HTTP endpoints implemented. Root reviewed worker detector and HTTP/test changes, fixed ambiguous Python parsing and test fixture/version errors. Real PG/private Gitea/Ollama discovery integration race6.262s passed. Remaining: scanner recovery/overlap tests, final source review, startup verification and commit.
- No paid calls or external repository writes. G5 remains open; T19–T28 required. Server currently75294 lacks latest discovery wiring until rebuild/restart. Environment/services unchanged; use ignored .local/development.env without printing secrets.

- T18 final root PG race3.483s passed; canonical scanner recovery/bot/conflict fixtures reviewed. Complete private discovery previously6.262s; focused detector/discovery/HTTP race and vet pass. Startup registers/routes/runs discovery, server9460 at8080. T18 local complete; T19 core repair and Luna recipe child now active.
- T17 latest production build and browser connection suite5passed, including actual private Gitea webhook issue/rotation/revoke and inventory; repository suite3passed3.0s after precise search selectors. Retried only affected tests. No live credentials printed; browser traces disabled for credential flows. Baseline display still awaits T19.

## Current checkpoint — T19 accounting and trusted execution

- Commits3bcd876 webhook-empty state, f6e77d8 T18 discovery,6a597fc T17 onboarding/inventory. T18 local complete. T19 active; T20 findings child in review, full parent still pending repair/run/change evidence. T21–T28 remain required.
- Root reviewed private model-turn broker and real-PG/Ollama acceptance; independent race6.078s passed. Added atomic unresolved-turn check, durable pre-dispatch reservation, encrypted replay, post-dispatch unknown holds, active job/connection cancellation. Unit race/vet passed before final accounting followups; migration014 applied dev/test. No paid APIs used. Pricing amounts use operator-pinned conservative rates; provider invoice reconciliation remains distinct.
- Uncommitted T19 recipe/validation/model broker and supervised engine under root review. Root owns all shared APIs/migrations/security; Luna owns findings UI, maintenance browser checks, and toolchain image packaging/runtime checks exclusively. No Astra workers.
- Remaining immediate checks: model broker concurrency/uncertainty regression, Go/Node/Python gVisor recipe execution, actual maintenance-save/discovery browser journey, engine protection regressions, full repair persistence/publication integration. G5 remains open; hosted production cgroup delegation and external credentials/certification absent.


## Current checkpoint — native repair publication

- T19 uncommitted: persistence/migrations015–016, frozen execution, scoped model/runner endpoints, app-owned staging and exact native commit validation before companion publication. Compilation passes; full service acceptance still pending. Native publication recovery, runner CLI and connected run evidence UI in progress.
- Reviewed commit-proof adapter source from Luna; tests reported pass, independent native proof acceptance pending. Real gVisor Go/Node/Python baseline-fail/source-patch-pass checks passed; Node/Python loader packaging fixed to resolve libraries with the image executable's actual ELF interpreter. Runtime now retains approved read-only image /home assets, HOME=/tmp; writable tmp quota follows configured disk allowance. Hosted cgroups still unavailable on this host.
- Disposable dev/test inventory/discovery tables were owned by bootstrap admin from earlier fixture setup. Restored ownership to existing migration role, then applied015–016; runtime role remains non-owner. Clean installation verification remains T26/T28.
- Maintenance browser reached inventory queue timeout before discovery assertions. Controller now has four bounded inventory workers; restart and real journey recheck pending. No fabricated browser success.
- Workers remain Luna only: CLI packaging, repair preview corrections, run evidence GUI. Root retains security review. T21–T28 required; G5 not passed.


## Current checkpoint — reviewed repair recovery

- Commit a957081 adds reviewed native commit proof adapters and Gitea operation markers. Real local Gitea native proof/idempotent publication passed2.35s; affected forge race tests passed independently1.161s/1.197s/1.145s.
- Root repair PG contract passed2.348s, then combined fresh-policy/repair regression3.186s after migration017: pricing changes invalidate previews, identical enqueue/report replay succeeds, changed idempotency body conflicts, artifact hashes bind to the active attempt, premature completion blocks, lost publication remains uncertain, authoritative recovery concludes with exactly one native publication. This contract uses explicit forge fixtures; it does not certify an external provider.
- Every native HTTP mutation now rechecks fresh current runner/policy/outbox authority. Private grants call controller status before each write; public writes use the same current check. Root affected race suite passed: repair1.070s, privateconnector9.734s, runnerclient1.095s, HTTP1.097s, runner2.514s, budget2.329s. Real gVisor isolation/cancellation passed8.669s after mount changes.
- Server93394 refreshed; log `.local/server-current.log`. Migration017 preserves native candidate artifact links, applied dev/test. Model0.6b engine acceptance reached actual gVisor/Ollama but did not produce a verified repair. Fixture HTTP timeout corrected; stronger free local model qualification underway. No successful mock substituted.
- Repair preview browser3pass; runs fixture1pass before expanded failure/SSE/narrow tests. Root fixed cached-list clearing, unused base64 decoding and private credential rotation race. Maintenance/discovery real browser recheck running. OpenAPI generation and full connected repair acceptance remain. T21–T28 required; G5 remains open.


## Current checkpoint — native evidence and discovery prerequisites

- Discovery StartScan now atomically queues missing inventory refresh; scanner defers until native evidence is fresh. Meaningful PG regression passed; real private-Gitea browser verification continues after correcting an overly broad text wait. No scan success claimed yet.
- Native-check failures persist their same-attempt artifact hashes and block publication. Repository baseline reads recheck repository scope; server derives unified source diffs from pinned source. Real PG repair regression passed2.019s: failed checks never publish, report replay remains idempotent, lost response reconciles one publication. Updated API generation passes; runner protocol race1.079s and frontend build/typecheck pass.
- Root fixed cancellation outcome, rejects malformed operator image registrations, and neutralizes control/bidi characters in rendered logs/diffs. T20 runs browser3pass before these display additions; baseline component is under root review and browser qualification.
- Actual local model/gVisor attempts remain unsuccessful: Qwen0.6b/1.7b incomplete repairs or malformed tool calls; Qwen4b timed out120s. Larger bounded timeout qualification underway, no mock substituted. Root corrected native-C fixture binding before next attempt. Full controller/runner/Gitea GUI repair acceptance still required.
- Workers Luna only: detectors owns engine integration test/documentation; connections owns private-Gitea browser and discovery-refresh regression; discovery_http owns RepositoryBaseline component/tests/doc. Root owns all security/backend/shared integration. T21–T28 remain required; G5 open.

- Subsequent combined private-transport/discovery/repair PG race suite passed5.159s. API generation required both string UUID and byte-slice upload mappings; generated package compilation and backend vet now pass. Root reviewed UI changes and fixed nil evidence arrays/full finding detail lists.
- Live discovery diagnosis: old failed browser fixtures left healthy connections pointing to stopped private runners; refresh jobs waited behind repeated unavailable reads. Logs now report bounded failure codes without raw provider errors. Historical fixture cleanup and failure-safe test cleanup underway; current server54694. No scanner success claimed.


## Current checkpoint — real repair and browser discovery verified

- Root actual complete controller/runner/Gitea test passed28.08s: real PostgreSQL, enrolled private runner, discovery, Qwen3:1.7b, gVisor H/H+patch/T+patch/C checks, same-attempt artifacts, one native PR and completed task. Log `.local/repair-live.log`; fixture removed. Current fixture repairs a CI regression; true dependency-upgrade acceptance remains to add.
- Root pure engine real model/gVisor test passed15.02s (`.local/engine-current.log`). Ollama tool turns now explicitly disable reasoning via documented protocol fields; other compatible profiles unchanged. Model broker budget/replay passed3.576s. No paid APIs used.
- Private Gitea browser lifecycle passed20.1s: enrollment, connection, webhook rotation, import, saved maintenance configuration, exact completed scan persisted on reopen, credential rotation/retest/revoke. Cleanup now scoped to created IDs. Full repair GUI journey next.
- Real runtime preparation and verify-runtime passed with approved local runsc. Config `/tmp/reforge-gui-1789911741776198591/runtime-config.json`; Go image registered for next server restart. Development isolation only; hosted cgroup qualification remains external.
- Root owns backend security, shared API and integration; Luna connections owns new live repair browser test, detectors owns Gitea namespace test hardening, discovery_http finished runtime packaging docs. All worker changes reviewed before integration. T21–T28 remain required; G5 open.


## Current checkpoint — isolated browser and protected merge

- Root remains sole Astra; three bounded Luna workers. Caveman-full. T21 started with fresh native merge inspection, guarded private operations and policy evaluator; persistence, qualification/configuration, queue control and GUI remain. T22–T28 remain required. G5 open.
- Committed repair accounting/recovery/API/runner and connected findings/runs/baselines. Latest `cf57ea3` adds immutable target-file reads and model retry when compatibility fails on target; focused race1.051s passed. Network/model deadline correction committed `01695e8`. Root UI fixture suite9passed1explicitlyskipped; cancellation recovery real-PG race2.796s passed.
- Actual dependency-upgrade acceptance remains failing: Qwen1.7b stopped without verified patch; Qwen4b timed out or returned uncertain tool protocol. Logs `.local/repair-upgrade-current.log`, `.local/repair-upgrade-target.log`, `.local/repair-upgrade-bounded.log`. No mock substituted. Next bounded run uses1.7b with new target-aware retry after browser releases local model. CI-regression repair previously passed28.08s.
- Automatic approval rejected browser setup that could leave authority in shared dev organisation. Safer approved harness `scripts/test-repair-browser.py` creates isolated DB/controller8081, seeds authority only there, then drops DB in finally. Actual browser acceptance running session3335; `.local/repair-browser.log`. Shared dev baseline untouched.
- Root owns shared APIs/migrations/security and browser authority/harness. Luna_connections owns repair-live.spec.ts/repair-fixture.ts investigation; Luna_detectors owns merge_inspection.go/test.go corrections; Luna_discovery_http performs read-only T16 wiring audit.


## Current checkpoint — merge persistence implemented, acceptance open

- T21 migration019 implemented/applied test DB. Repository configuration, immutable preview snapshots, guarded durable operations, per-target serialization, cancellation/reconciliation and HTTP wiring implemented; real native controller acceptance, queue/train final gates/cancellation, automatic reconciliation and bot ordering remain. Operator attestations do not equal Reforge certification; see t21-merge.md.
- Root reviewed Luna inspector/transport/policy tests; fixed queue check repository and shared test pointer alias. Root affected race contracts passed: providers1.042s, privateconnector9.397s, mergecontrol1.050s, Gitea1.108s. Real PG isolation/cancellation/restart test passed1.353s. Latest stricter snapshot identity/hash validation also passes focused race.
- Changes GUI connected in SectionPage. Root review required correct endpoint, CSRF, `allow` outcome, stale selection reset, stable idempotency, expiry and role controls. Luna fixture browser2pass; full root review/real endpoint journey pending.
- Actual dependency-upgrade run with target-aware1.7b failed26.53s: model stopped after2turns without a verified patch (`.local/repair-upgrade-compatible.log`). Next diagnose model/tool response before further retries. Browser's previous5second assertion raced the native preview; now waits actual response. Root isolated rerun session48966 uses correct registered image and `.local/repair-browser-current.log`. No local acceptance success invented.
- Root owns all backend/shared files and integrated browser helper; Luna workers currently completed. Main development server remains54621 with older binary; currentmerge code not yet built/restarted there. G5 open; T16/T19–T28 outstanding work retained.

## Current checkpoint — merge recovery and model diagnosis

- T21 commits `b8beb55` separates protection inspection; `fe3f775` persists guarded merge operations. New operation history carries requested and refreshed gate IDs separately. Root found/fixed GUI recovery binding errors, inspector-version binding and publisher editing bugs; configuration browser qualification still underway. Changes browser8pass reported by Luna; root review pending.
- Migration020 adds bounded, fair observation scheduling. Background merge observer reads canonical outcomes after restart without repeating writes; native queue cancellation adapters under Luna development. Queue/train final execution gate, bot ordering, complete native controller acceptance and shared API generation remain. Root sole Astra; three Luna slots maximum.
- Focused protection/provider/merge race tests pass12.752s/1.084s/1.075s; real PostgreSQL merge isolation/recovery contract pass1.530s. New observer migration applied disposable test DB. Dev DB/binary still needs current migration/build/restart.
- Dependency-upgrade acceptance remains failed: Qwen2.5:3b completes tool turns but generates invalid repair,157.25s (`.local/repair-upgrade-protocol.log`). Qwen3:4b returns precise private failure `protocol`, uncertain=true, zero completed turns,106.99s (`.local/repair-upgrade-qwen4-final.log`). This is protocol failure, not proven transport timeout. Bounded adapter diagnosis underway; no successful mock substituted.
- Isolated real browser repair failed54.4s at model handoff (`.local/repair-browser-current.log`); baseline executed, no verified candidate. Python harness now preflights image digest and kills disposable process groups; TypeScript cleanup bounded and unconditional. Root review pending. No shared development authority modified.
- Ownership: root backend/security/shared contracts/integration; Luna_connections MergeSettings and focused browser tests; Luna_detectors new GitHub/GitLab queue cancellation adapters/tests; Luna_discovery_http read-only model diagnosis document. T16/T19–T28 remaining implementation retained; G5 open.


## Current checkpoint — real protected merge verified

- Root real controller/runner/Gitea protected merge passed49.55s (`.local/repair-protected-merge.log`): actual local model repair and native publication, missing-review denial, real native approval, stale-configuration rejection, strict protected fast-forward merge, identical-request replay and cross-tenant denial. Separate inspector and non-admin merge actor used. This certifies only disposable Gitea1.27.3 fixture configuration.
- Latest affected race checks pass: mergecontrol1.060s, privateconnector9.363s, GitHub/GitLab/providers cached; real PostgreSQL merge history/cancellation/restart/isolation1.737s. Observer contract exercises persisted revocation intent; canonical background completion contract remains to add. Migrations020/021 applied dev/test.
- Commits83e061f and ea14438 settle known truncated-response usage and bound text-only continuation. Real dependency upgrade Qwen3:8b failed159.51s at eight-turn limit (`.local/repair-upgrade-qwen8.log`); upgrade candidate passed but target compatibility remained invalid. No dependency acceptance or full browser repair success claimed.
- Queue cancellation and background observation implemented. Queue admission/final execution gate remains disabled pending controller implementation and qualified native enforcement. Bot companion ordering/revalidation also remains. T16 wiring and T22–T28 retained. G5 open.
- Root sole Astra; Luna_detectors checks current Changes/settings browser; Luna_connections observer test review complete with canonical completion gap; Luna_discovery_http researches exact native queue gate APIs. Server85413 contains refreshed embedded GUI.


## Current checkpoint — merge GUI integrated; repair wire diagnosis

- Commits c593b05 native queue cancellation,1f361eb GitHub execution checks,a777c2b durable merge recovery,c02daf3 companion ordering,526d06d connected merge GUI/API. Root reviewed worker changes; corrected GitHub nested repository parsing and GitLab absent-train state checks before integration. Latest affected races pass GitHub1.219s,GitLab1.125s,privateconnector9.421s,providers1.046s. Generated API consistency and backend vet pass.
- Current merge browser18passed8.3s (`.local/merge-browser-root.log`), including publisher deletion, disabled/no-inspector configuration, non-Gitea configuration, version conflict, recovery, org switching, pagination and companion states. These browser provider responses are fixtures; real protected Gitea controller test previously passed49.55s. Server22483 contains current GUI/backend; recipe v2 and latest engine need rebuild for the browser.
- Real full browser with Qwen2.5:3b failed at model handoff (`.local/repair-browser-qwen25.log`). Input-dependent live tests now use multiple inputs; no constant-output workaround. Native restart-fault variant has not yet reached its new assertion because model repair fails.
- Raw local model capture proves correct history and complete seven-turn stream in Qwen1.7b61.02s failure: incorrect model patch, not parser failure. Capture `.local/model-capture/test-1795855146`; opt-in helper `model_capture_test.go` reviewed/fixed to forward probes and synchronize capture. Qwen8b eight-turn upgrade failed106.67s; logs/capture preserved. No repair success substituted.
- Commit2911c04 removes duplicate validation/log injection on read-only turns; engine race1.056s passes. Recipe v2 now sets12-turn ceiling; per-turn budget/time checks remain hard stops. New live qualification pending. Fixture request allowance explicitly12, no paid APIs.
- Root remains sole Astra. Fresh Luna_queue_candidate owns only GitHub merge.go/new queue_candidate_test.go, proving documented headCommit with exact H/T parents before exposing C. All other workers idle/completed. T21 final gate/admission, bot automatic revalidation, T16 production custody/wiring and T22–T28 remain. G5 open.


## Current checkpoint — real dependency upgrade passed

- Commits b818160 prove GitHub native queue candidate H/T parents;8308a68 introduced recipev2. Its12 turns exhausted after correct diagnosis. Recipev3 with16 bounded turns passed the actual controller/runner/Ollama/Gitea upgrade in153.769s (`.local/repair-upgrade-v3.log`); no paid APIs or model mocks. Baseline, both dependency revisions, native candidate and publication checked.
- Real Go browser still failed16 turns without a candidate (`.local/repair-browser-v3.log`). Luna added opt-in bounded local wire capture; root review/rerun pending. Protected restart fault injection still needs its full new run.
- T21 GitHub queue prerequisites and distinct admission/execution policy contracts pass race1.195s/1.049s. Root implemented durable fenced check intents and background final-candidate checking; migration022 applied dev/test. Controller PG contracts, cancellation races, GUI/API integration and review remain before marking queue work locally complete. GitLab final gate and bot revalidation remain required.
- Root sole Astra; Luna owns browser capture and new queue PG contract tests. T16 production wiring, T22–T28 remain. G5 open.


## Current checkpoint — real GUI and restart recovery passed

- Commit3f39ac2 stops repeated forbidden test rewrites with precise feedback; focused engine regression passed1.055s. Frozen validation remains unchanged.
- Actual Go GUI repair passed1.8m (`.local/repair-browser-input-diagnostics.log`) with local model/private runner/gVisor/Gitea, immutable input-dependent tests,390px/focus/keyboard/artifacts and exactly one native publication. Dependency-upgrade acceptance already passed153.769s. Browser harness/docs pending commit.
- Real protected Gitea merge plus injected lost persistence/restarted canonical observation passed55.003s (`.local/repair-protected-restart.log`). This qualifies the disposable server profile only.
- Queue PG admission/C gate/pause/cancellation/uncertainty/restart/RLS contracts passed2.625s (`.local/queue-pg-pause.log`). Worker cancellation fixture was corrected to native not_queued contract; reported cross-tenant failure used the same globally identified owner in its own organisation, corrected to an actual other-tenant scope. No production bypass demonstrated.
- Queue policy now blocks unknown/locked/unmergeable states. Affected races pass (`.local/queue-final-contracts.log`). GitHub final gate/migration022 implemented, final UI review pending; initial browser19pass2fail (old label and390px overflow). Luna_browser_capture owns only scoped Changes CSS/tests fix; Luna_discovery_http finished read-only GitLab manual-job API research. Root owns backend/security/shared files.
- GitLab train gate remains disabled pending exact native pipeline/job identity and qualified protected blocking manual job. T16 production wiring and T22–T28 retained; G5 open.

## Current checkpoint — train gate implemented, acceptance underway

- Commits44d2eee GitHub queue controller,308dc48 GitLab cancellation contracts,f520cc4 queue GUI; GitHub/settings browser21passed9.3s. T19 actual GUI and protected restart committed8c43527/cd3617b.
- GitLab final train gate now implemented locally: protected blocking native job, exact pipeline/C/CI configuration, durable fenced play, no repeat after uncertainty, canonical merge observation and queue cancellation. Root reviewed Luna adapter/UI/PG tests and corrected fixture queue IDs, cancellation convergence and GUI numeric validation. Pending integrated browser/review before commit.
- Root affected race contracts pass (`.local/train-race-fixed.log`); explicit native HTTP drift/approval scenarios pass1.141s (`.local/train-drift.log`); real PostgreSQL combined merge/queue/train contracts pass3.347s (`.local/train-pg-root.log`). These do not certify external GitLab.
- Auto-review rejected removing read-time mergeGuard blockers. Safer alternative retained those blockers and binds inspection to operation-specific guards that reject every merge mutation; approved and contract checks pass. No guard requirement removed, no unresolved approval request.
- Current local controller session46771 on8080; train browser session52180 (`.local/train-browser-root.log`). Root sole Astra; Luna All Luna workers idle after T24 review; root owns shared integration. Root owns all current files. Automatic original bot revalidation remains next T21 work; T16 production custody/wiring and T22–T28 remain required; G5 open.


## Current checkpoint — T21 local acceptance; T22 started

- Commits a5bfffc/b2297db implement qualified GitLab train gate and connected configuration. Native-rule refresh guards retained; affected races and 23 browser checks pass. External GitLab certification still required.
- Commit2b11b69 adds automatic original bot revalidation after canonical companion merge, fresh H/T/native checks and current author authority. Root fixed stale ready state after author revocation. Real PostgreSQL contract passes1.749s, `.local/bot-pg-root.log`; provider writes remain zero.
- Root reviewed bot GUI and Luna fixtures. Combined bot/Changes/settings22passed10.3s; added meaningful retry, missing gate, clock expiry, nonempty390px/keyboard cases; bot7/7pass. `.local/bot-browser-luna.log`. UI commit follows.
- T22 contracts and GH/GL read adapters implemented under review. Root owns deployment native gates/dispatch/controller/shared transport and migrations. Luna_discovery_http owns GH delivery_read.go/test.go; Luna_queue_pg_contract owns GL equivalents; Luna_browser_capture bot browser finished. No Astra workers.
- Source/digest/config/environment pins, native gates, durable single dispatch and authenticated provenance/health required. Gitea uses protected GitOps delivery. T16 wiring, T23–T28 remain authorised; G5 open.


## Current checkpoint — deployment review and acceptance

- T22 uncommitted: GitHub/GitLab guarded dispatch, exact workflow/correlation observation, native gate inspection, recovery workflow, signed provenance/health, durable per-environment intent, explicit/pause cancellation, API and connected settings/deployment GUI. Migrations024–026 applied dev/test; do not edit applied migrations. Root fixed late-response overwrite races and invalid JSON settings saves.
- Current full affected races pass: policy cached, GitHub1.181s, GitLab1.198s, privateconnector9.371s. Log `.local/deployment-final-race.log`. Generation and GUI/server builds pass; latest server57966. Native cancellation HTTP contracts pass separately. Real PG cancellation/recovery and browser tests running; no new pass claimed.
- Workflow SHA differs from artifact source SHA; both pinned, reserved workflow SHA input permits a qualified workflow to enforce its own revision before side effects. Health requires signed source/digest/run/revision, fresh criteria and elapsed observation window. Native approval APIs never invoked.
- T23 pure deterministic YAML/JSON field patcher delegated to Luna; source/delivery authorization, protected merge integration and health remain root work. Root sole Astra; three Luna maximum. T16, T17 qualification flow and T23–T28 required; no G5 claim.


## Current checkpoint — T22 locally complete; T23 implementation

- T22 commits `2e44024` native adapters, `9e7db69` durable promotion/recovery, `387dd88` API. Observe-only import and GUI pending final bounded commits. Native/provider transport races pass; real PostgreSQL deployment/recovery/cancellation4.289s and observe import4.499s pass. Browser13/13 pass2.9s (`.local/deployment-observe-browser.log`), including real development empty state,390px and keyboard. Last finite-date guard typechecks; next full build refreshes embedded GUI.
- Observe-only import verifies canonical source, signed provenance, exact configured workflow/run; zero provider writes; no cancellation authority. Migration028 enforces unique native run identity. Migrations027/028 applied dev/test; do not edit them. Offline Ed25519 evidence signer implemented and controller interoperability passes; docs being updated. No live GitHub/GitLab deployment certification claimed.
- T23 parser reviewed/fixed exact UTF-8 positions, deterministic semantic diff, immutable image digest, duplicate/tag/alias/multiline rejection. Parser/merge-package/signer race checks pass1.055s/1.054s/1.052s. Root implements configuration, pinned dual-repository policy/provenance preview, durable branch/PR phases, candidate tree/commit verification and merge authority hook. These are uncommitted and not yet integration-verified. Observer, health, protected merge wrappers, API/wiring and real Gitea acceptance remain.
- Root sole Astra; Luna_queue_pg_contract owns GitOps API client/page; Luna_discovery_http owns T22 operator doc; Luna_browser_capture completed T22 tests. Current server65286, before latest qualification-date UI guard. Other full backlog, T16 production custody and T23–T28 retained. G5 open.


## Current checkpoint — protected GitOps flow and recovery verified

- T22 final bounded commits `b725e69` observe-only import, `d4f7596` connected GUI, `2a4c7a1` offline evidence signer/operator contract. T22 remains locally complete, live GitHub/GitLab native approval certification open.
- T23 root service/API implemented: immutable dual-repository preview, signed provenance, deterministic field patch, independent candidate commit/tree proof, durable single stage/publication dispatch, canonical restart reconciliation, protected merge hook, source/delivery revocation checks, signed reconciler health, separate known-good recovery proposal. Migration027 already applied; generated API updated. No direct Kubernetes operations.
- Real disposable Gitea/private-connector/PG protected workflow passed13.031s (`.local/gitops-live-root3.log`). Signed health is a fixture observer contract; no external Argo/Flux runtime certified.
- Focused GitOps/merge races pass1.053s/1.049s; PG scope/health2tests passed (worker). Pure patcher5s fuzz117,988executions passed (worker, root reviewed tests). Root added physical-target consistency to recovery after live pass; final affected rerun pending.
- Root rejected incomplete Luna GUI twice; fixed first/second environment creation, stale state, role actions, source vs delivery identities, merge types/method/expiry, idempotency and history. Fresh controller49849 on8080 includes GUI; parser/manifest-proof backend changed since that build. Luna owns bounded browser contracts and formatting only; root owns service/shared/API.
- T23 browser/review/commits pending. T16 production subscription custody/wiring, T17 qualification GUI and T24–T28 remain authorised. G5 open; no release claim.


## Current checkpoint — T23 local acceptance complete

- Real PostgreSQL/Gitea GitOps suite passes29.167s (`.local/gitops-final-pg-native2.log`): deterministic concurrent stage claim, lost branch/publication responses, exact original publication recovery, source revocation through direct merge API, protected native approval/merge, restart, forged/wrong/replayed signed health, separate failed rollout and known-good revert. Original native history retained. Rejected private dispatch can transiently consume one readiness poll; bounded read-only observation retries in fixture, no repeated writes.
- Root fixed concurrent losing-request persistence with stored dispatch identity, bounded lexer tokens/nesting, immutable post-merge manifest proof, and recovery physical-target consistency. Parser/API/merge focused races pass (`.local/gitops-parser-depth.log`). Current controller12389 has these production changes. Later changes affect tests/docs only.
- GitOps browser6/6pass4.5s (`.local/gitops-browser-root2.log`): configured/empty views, role/preview/retry/native blockers, keyboard, strict390px document width and actual unmocked authenticated empty page. Fixture login initially raced redirect; corrected. Scoped `.gitops-page` layout fixes genuine horizontal overflow. Browser fixture coverage is separate from real native backend acceptance.
- Commit50578bc bounds parser complexity. Remaining T23 bounded commits are being recorded. Operator procedure `t23-gitops.md`; decisions recorded. T23 local dependency complete; external GitHub/GitLab protected promotion and live GitOps reconciler health certification remain open.
- T16 production custody/wiring, T17 remaining qualification flow, T24–T28 retained. T19/T20 acceptance/status reconciliation next. G5 open.

## T24 checkpoint — 2026-09-21

- Three Luna drafts reviewed. Root corrected policy null/inherit versus empty/deny-all lists, restrictive-policy activation, first binding CAS0, stale drafts, owner-only budget writes, immutable budget periods and audit page export.
- New tenant/repository-scoped usage ledger/summary and audit/export APIs. Stable tuple pagination; settled pricing estimates separate from unknown maximum holds. Org audit restricted owner/admin; repository audit uses current bindings.
- PostgreSQL/HTTP race acceptance passed1.127s (`.local/insights-pg-root3.log`): scoped reads, actor/action/provider/recipe/state/time filters, unknown holds, cursor ties, tenant/team denial, revoked session/export. API/SQL generation and frontend production build passed. Browser acceptance underway; no completion claim yet.
- T23 commits: 50578bc parser bounds,9763708 source authority,afc6fb0 protected promotions,6e532b0 API,b45b591 GUI. T23 final actual PG/Gitea suite29.167s and browser6/6 recorded above.
- T25 requirements located; no implementation yet. T16 custody/runtime wiring and T17 qualification GUI remain, followed by T25–T28. G5 open.

- T24 final7/7 browser checks passed4.3s (`.local/insights-browser-final.log`), including unmocked real policy activation, persisted budget reload and actual downloaded audit NDJSON at390px; keyboard and primary-team regression included. Backend commit b470fff; GUI report `t24-controls.md`.
- T19/T20 reconciled to local complete using previously recorded real repair/Ollama/Gitea/gVisor publication and browser evidence. T16 official subscription runtime production wiring remains independently incomplete; no external certification inferred.

## T25 checkpoint — 2026-09-21

- Root froze campaign domain/HTTP contract in `t25-contract.md`, types in `internal/campaign/types.go`. No campaign service/GUI implemented yet. Repair/pipeline/GitOps reuse existing execution controllers; fixed membership and representative canaries; fresh authority and pinned policy checks before dispatch; native approvals unchanged.
- T24 GUI commit53b315a, backendb470fff. All7browser scenarios, PG race, vet/build/generation passed; report `t24-controls.md`.

- T25 draft implementation: migration029 (NOT APPLIED), root types/rules/metadata snapshot, campaign API client; two Luna GUI drafts returned and await full root review. Only compile checks run so far; no T25 acceptance claim. Root owns all backend/shared files. Campaign dispatch/controller/authority hooks/API integration, migrations and tests remain.

- T25 root added scoped preview/create/list/detail/member reads and draft control transactions plus workflow campaign pause propagation. Native executor/controller, operation admission hooks, resume/reconciliation, API/startup, migration application and meaningful acceptance remain unimplemented. Metadata snapshot compile passed; not a production completion claim. Two Luna GUI drafts exist; form requires further native member/window review. No T25 commits yet.

- T25 pause/admission groundwork now changes workflow/repair and pipeline/GitOps services via startup-registered hooks (not yet registered). Root added internal non-serializable, repository-scoped maintainer automation grants for scheduled work surviving browser logout; interactive campaign management rejects grants. Auth proof is delegated only through trusted in-process callbacks, never request JSON. Exact PG acceptance result in `.local/campaign-automation-pg.log`; review before claiming it passed.
- Running server2619 remains T24 build. Migration029 still NOT APPLIED. Native executor/controller/hook implementations and integration tests are still required. Workers returned GUI changes; root review remains.

- Scoped automation grant PG acceptance passed1.103s; complete affected identity/CSRF/OIDC/concurrent-revocation suite passed3.170s (`.local/campaign-identity-regressions.log`). Auth subfeature committed; campaign callbacks/controller still under implementation. Added draft campaign gate/task authority callbacks; not wired into running server.

- T25 root controller/executor/API/startup now drafted and compile: persistent campaign lease, one admission per turn, rotating tenants, representative stages, protected native delegation, operation binding before effects, explicit resume/grant expiry, pause/cancel propagation and canonical outcome checks. Migration029 applied to disposable dev/test databases; do not edit it. Repair member and native gate authority hooks registered at startup. New build not yet running.
- T25 PG controller simulations passed3.764s (`.local/campaign-pg5.log`): failed/unknown canaries stop expansion, bounded progression, fixed membership, changed connection pins, pause during pending dispatch, stale grant fencing, initiating-role revocation and grant expiry. These use an explicit simulated executor and do not certify production integration.
- Native pipeline known-undispatched continuation implemented with persisted dispatch CAS. Root rejected initial worker test that cleared a successful dispatch marker; revised test seeds requested intent before effects, passes PG race1.592s (`.local/deployment-resume-race.log`) with at-most-once concurrency, uncertain outcome no-repeat, viewer no-write checks. Root final review/integration suite pending.
- Root corrected Luna GUI drafts: independent native member provenance, advanced JSON without manual selection, environment append behavior, weekly UTC windows, qualified route lookup, role gating and budget navigation query retention. HTTP adapter and pure rules tests reviewed; API generation passed (`.local/campaign-generate.log`). Browser/real campaign executor acceptance, isolation/fairness/budgets/recovery tests, final review and commits remain. T16/T17 gaps and T26–T28 remain; G5 open.

## Functional-demo checkpoint — 2026-09-21

- Luna owns the implementation changes; root reviews the integrated result. The current self-hosted demo uses the disposable local PostgreSQL/Go service and authenticated browser workflow; it does not claim external provider or production certification.
- T24 controls are locally complete: seven browser scenarios passed in `.local/insights-browser-final.log`, with backend evidence in `.local/insights-pg-root3.log`.
- T23 protected GitOps is locally complete against disposable Gitea: PG/native evidence is in `.local/gitops-final-pg-native2.log`, with browser evidence in `.local/gitops-browser-root2.log`.
- T25 remains in progress. Migration029 is applied to disposable dev/test databases. Pre-dispatch cancellation passed in the targeted race; final native observe/change, final observation window, campaign browser lifecycle, and fuller review remain. T16 custody/runtime wiring, T17 qualification, and T26–T28 remain deferred. G5 remains open.
- Reported campaign evidence awaiting final review: `.local/campaign-fairness1.log` (8.100s), `.local/campaign-repair2.log` (2.290s), and `.local/campaign-browser2.log` (6/6, 2.5s). `.local/campaign-browser-live1.log` is a failed attempt superseded by `campaign-browser2.log`. Latest native observe/change, final-window, and pre-dispatch cancellation review remains pending.

## Frozen demo handoff — 2026-09-21

- Current main service is ready at `http://127.0.0.1:8080`; retained repair service is ready at `http://127.0.0.1:8081`; both `/readyz` endpoints returned HTTP 200 and the controller/runner remained alive.
- Final retained repair run is blocked after the model stopped without a candidate or PR: [b1cca48f-eab5-4ee3-adac-0a7873743926](http://127.0.0.1:8081/org/00000000-0000-4000-8000-000000000001/runs?run=b1cca48f-eab5-4ee3-adac-0a7873743926). No verified repair result is claimed. Earlier successful repair evidence remains historical only.
- Backend targeted race passed17.740s in `.local/frozen-targeted-race-final.log`, including pre-dispatch cancellation and startup callback ordering review. Harness commit `c110fe4` retained the cleanup/recreate path.
- T25 remains partial and uncommitted pending fuller review. T16/T17/T26–T28 remain deferred; G5 remains open.
