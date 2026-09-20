# Implementation backlog

Updated: 2026-09-20. All tickets are `not started`. Execute only after the owner invokes implementation. Planning artifacts and the synthetic prototype are already present; production code is not.

## Working rules

- R01–R13 in `../../PLAN.md` are release requirements. No worker may silently defer one.
- Root/coordinator owns shared contracts, module/package manifests, migrations ordering and integration branches.
- Astra owns ambiguous/security-sensitive architecture, tenancy, execution, protection/merge and deployment work. Luna owns bounded adapter/UI/recipe work after contracts are frozen. Every Luna change receives root/Astra review.
- A ticket is complete only with implementation, its stated functional verification, documentation update and a reviewable diff. Mocks alone cannot certify a real forge or subscription entitlement.
- Small bounded commits per ticket; preserve unrelated user changes. No deployment, publishing a public repo or real customer mutation is part of local build tickets.
- Source comments and README files are not created unless explicitly authorised by the owner; use named types, clear structure and the existing docs. Do not add trivial tests to inflate coverage.

## Dependency waves

| Wave | Tickets | Parallelism |
| --- | --- | --- |
| W0 | T01 contracts/foundation | Astra; no conflicting foundational edits |
| W1 | T02 identity/tenancy, T03 secrets, T04 GUI shell | One Astra plus up to two Luna workers where ownership is disjoint |
| W2 | T05 workflow/budgets, T06 policy, T07 isolated runner | Security-critical Astra tasks; execute sequentially if only one Astra slot is available |
| W3 | T08 GitHub, T09 GitLab, T10 Gitea, T11 event reconciliation | Adapters parallel after T01 interfaces; shared ingestion wiring after adapter contracts |
| W4 | T12 OpenAI, T13 Anthropic, T14 Google, T15 compatible inference, T16 official agents | API adapters in separate directories; agent bridge Astra; freeze dependency manifests via root |
| W5 | T17 portfolio GUI, T18 discovery/bots, T19 repair/validation, T20 evidence GUI | Interface-backed GUI can run alongside backend tickets; integrate one full vertical slice early |
| W6 | T21 merge controller, T22 direct CI/CD, T23 GitOps delivery, T24 control GUI | Certified APIs and full tests; no parallel ownership of state transitions |
| W7 | T25 campaigns, T26 distribution/operations, T27 isolation/recovery qualification, T28 release qualification | Integrate, benchmark and qualify before release |

Waves describe coordination, not permission to ignore exact dependencies below. Cap active workers to the available slots, normally root plus three. Root reviews/integrates continuously; do not wait until every branch is finished.

## Tickets

### T01 — Foundation and contract freeze

- Owner: Astra. Dependencies: none. Requirements: R01, R02, R03, R12, R13.
- Own: `go.mod`, `go.sum`, `cmd/server`, `internal/httpapi`, initial domain interfaces, `api/openapi.yaml`, migration baseline, frontend manifest/build setup, Apache-2.0 licence/third-party notices, dev commands.
- Build: Go+Gin health/readiness/API skeleton, graceful shutdown, explicit trusted proxies/body/time limits, structured errors, OpenAPI types, PostgreSQL migration tool, frontend served by Go, local fixture mode visibly labelled.
- Decide/pin module namespace locally (`reforge` until remote selected), dependency versions, status enums and generated-code ownership. Do not copy AGPL competitor implementation into Apache-2.0 source; reuse concepts and compatible dependencies only.
- Accept: clean machine builds server/frontend; migration applies to empty DB; API/type generation has no drift; normal application cannot enable fixture auth/data accidentally.

### T02 — Organisation identity and scoped authorisation

- Owner: Astra. Dependencies: T01. Own: `internal/auth`, tenancy queries/migrations, membership endpoints.
- Build: OIDC sessions, logout/revocation, organisation/team/repository roles, self-hosted bootstrap, server-side scoped lookups, CSRF and secure cookies, audit actor context.
- Accept: cross-tenant and cross-team scope-resolver/API contract tests cover enumeration, details, artifacts and SSE; pooled DB connections cannot retain another tenant's context; runtime role cannot bypass RLS. Exercise actual artifact endpoints and SSE session revocation when those transports land in T05/T07, with integrated regression in T27.

### T03 — Connections, credentials and private routes

- Owner: Astra. Dependencies: T01, T02. Own: connection services, secret storage, network destination validation.
- Build: envelope encryption, write-only secrets, rotation/revocation, tenant-bound forge/model/pipeline connection records, approved private routes and CA configuration, capability timestamps.
- Accept: no secret in JSON/events/logs; redirect/DNS-rebinding/metadata destination tests fail; approved private-route records and fixed-operation transport contract work against fixtures. Live private Gitea/model connectivity on an enrolled runner is verified in T07/T11/T15 and T27, after the runner exists.

### T04 — GUI shell and design system

- Owner: Luna, Astra reviews authorisation assumptions. Dependencies: T01. Own: `web/src/app`, `web/src/components`, styles/tokens.
- Build: shell/navigation, organisation scope, URL filters, query client, status/gate components, accessible tables/dialogs, standard empty/loading/error/stale states. Use prototype as visual direction, GUI spec as authority.
- Accept: keyboard navigation and responsive shell; deep links/back restore state; organisation switch clears cached tenant data; all demo data confined to explicit design/development fixtures.

### T05 — Durable jobs, events, budgets and reconciliation

- Owner: Astra. Dependencies: T01, T02, T03. Own: `internal/workflow`, usage ledger, outbox, SSE transport.
- Build: transaction-backed states, fair job leases/fencing, heartbeats, stable operation IDs, bounded retries, event replay, atomic hierarchical budget reservation and unknown-usage accounting.
- Accept: kill/restart at dispatch/result/outbox boundaries; late stale worker results rejected; 100 competing reservations cannot overspend a configured ceiling; no duplicate operation intent; SSE replay preserves scope/order and revoked sessions lose access.

### T06 — Deterministic policy engine

- Owner: Astra. Dependencies: T01, T02. Own: `internal/policy`, policy schema/versioning and simulation endpoints.
- Build: deny precedence, scope intersections/minimum caps, multi-team defaults, typed conditions, action explanations, current-policy revalidation, pause semantics.
- Accept: policy corpus from `../policies.md`; candidate repo policy cannot activate itself; unknown never allows a mutation; lower scopes cannot weaken higher constraints; simulation emits no side effects.

### T07 — Isolated runner and tool broker

- Owner: Astra. Dependencies: T03, T05, T06. Own: `cmd/runner`, sandbox runtime and constrained execution tools.
- Build: outbound enrolment/leases, per-job credentials, pinned checkout, disposable workspaces, sandbox command execution, separate model/auth process and repo commands, resource/network restrictions, artifact capture/cleanup, cancel process groups.
- Accept: malicious hooks/symlinks/tests cannot reach host files, auth stores, DB, metadata or another tenant; stale fence cannot upload/publish/call broker; cancellation terminates descendants; enrolled private-route transport works without a general proxy; artifact reads obey T02 scopes. Hosted sandbox isolation gate passes before hostile repos are admitted.

### T08 — GitHub adapter

- Owner: Luna for inventory/webhooks/PR CRUD; Astra for protection and merge portions. Dependencies: T01, T03. Own: `internal/forge/github`, provider fixtures.
- Build: App installation auth and supported GHES endpoints, paginated inventory, verified webhook decoding, CI/check publisher identity, branch protection+rulesets, bot source detection, guarded PR publication, native merge/queue capability.
- Accept: revoked installation/insufficient scopes/rate limit paths; same-named spoofed check rejected; protected branch and queue fixtures plus real test-org certification; no bypass/force push path.

### T09 — GitLab adapter

- Owner: Luna for inventory/MR CRUD; Astra for protection/train portions. Dependencies: T01, T03. Own: `internal/forge/gitlab`, provider fixtures.
- Build: cloud/self-managed API/auth, subgroup inventory, token/signed webhook verification as version supports, detailed merge/approval states, pipeline and train data, guarded MR publication/merge requests.
- Accept: pagination/subgroups; calculating/unknown approval state blocks; exact source SHA conflict; protected environment tracking never treated as enforcement; certify target-freshness/train path on supported versions/tiers.

### T10 — Gitea adapter

- Owner: Astra until capability/protection contract is proven; Luna can implement bounded CRUD. Dependencies: T01, T03. Own: `internal/forge/gitea`, fixtures and local Gitea test deployment.
- Build: configurable instance/OpenAPI capabilities, scoped identity, signatures, repos/PR/checks/protection, provider-specific CODEOWNERS parsing/enforcement visibility, exact-ref publication and guarded merge.
- Accept: real local Gitea tests for head movement, outdated reviews, required checks, force=false and strict target updates; unsupported features visibly block only affected actions; no GitHub API-shape assumptions.

### T11 — Inventory, webhook intake and polling

- Owner: Luna with Astra review of tenant routing. Dependencies: T05, T08, T09, T10. Own: inventory/reconciliation orchestration and webhook route wiring.
- Build: incremental/full sync, bounded queues, replay/delivery dedup, authoritative state refresh, access removal, archived repository handling and rate-aware polling.
- Accept: duplicate/out-of-order/missing webhooks converge to provider truth; incomplete pagination is stale/unknown; repository rename preserves identity; 1,000-repo import remains asynchronous. Verify private Gitea connectivity through T07's enrolled runner in the integrated qualification suite.

### T12 — OpenAI direct API adapter

- Owner: Luna; Astra reviews streaming/tools. Dependencies: T01, T03, T05. Own: `internal/model/openai`, stream fixtures.
- Build: official Go client where appropriate, configured supported API surface, streaming tools/usage/continuation, cancel, capability probe, bounded retries and errors.
- Accept: split arguments, parallel call IDs, interruption/late usage, unsupported model and quota exhaustion; never execute partial arguments or silently change billing route. Recheck official docs at build time.

### T13 — Anthropic direct API adapter

- Owner: Luna; Astra reviews. Dependencies: T01, T03, T05. Own: `internal/model/anthropic`.
- Build: native Messages/tool/stream parsing, official client, supported opaque continuation, API authentication, connection capability probe.
- Accept: indexed content blocks, partial JSON, tool errors, rate limit, cancellation, reported/unknown usage; no Claude subscription token accepted as API key.

### T14 — Google direct API adapter

- Owner: Luna; Astra reviews. Dependencies: T01, T03, T05. Own: `internal/model/google`.
- Build: official Go Gen AI SDK, pinned supported GenerateContent surface, required thought-signature preservation, customer API/project credentials, tool/result events and capability probe.
- Accept: multi-turn tool continuation fixtures, quota/project/auth failures and cancellation; never read Gemini CLI OAuth caches or identify as its OAuth client.

### T15 — Self-hosted compatible inference

- Owner: Luna; Astra reviews endpoint security. Dependencies: T01, T03, T05. Own: `internal/model/compatible`.
- Build: OpenAI Chat Completions protocol profile; independently probed Responses option, configurable model IDs/context, Ollama/vLLM qualification and runner-bound private networking.
- Accept: one real compatible endpoint per supported profile; malformed/unsupported tool output fails safely; unknown usage and absent model-list API handled; no hosted SSRF exception outside approved route. Qualify private model transport through T07's enrolled runner during integrated qualification.

### T16 — Official agent bridges and entitlement qualification

- Owner: Astra. Dependencies: T03, T05, T06, T07. Own: `internal/agent`, pinned optional runtime packaging and support matrix.
- Build: Codex documented app-server bridge first; Claude official binary bridge and Gemini CLI only under qualified terms/topology. Native approval handling, cancellation/resume and isolated-command support mandatory for mutation.
- Accept: actual runtime version tested for auth/logout/revocation/quota, approvals before effects, credential custody and shell isolation. An approval request is not simulated from a post-execution event. Subscription route status/billing visible; no API fallback without explicit permission/budget.
- Gate: eligible documented subscription/workspace route preferred when available; uncertain route disabled with reason. Direct API support for all required model families remains mandatory. No unsupported promise of universal subscription access.

### T17 — Portfolio and connection GUI

- Owner: Luna. Dependencies: T02, T03, T04, T11; model forms depend on T12–T16 capabilities. Own: repository/onboarding/connection/runner list routes.
- Build: import wizard for all forges, repository detail/baseline, scoped filters/saved views, connection health/model billing-route/capabilities, write-only rotation and enrolment UX.
- Accept: J01/J08; keyboard onboarding across three providers, private-route failure recovery, stale/unsupported capability states, no exposed credential values.

### T18 — Finding discovery and dependency-bot coordination

- Owner: Astra defines ownership/dedup; Luna implements bounded detector recipes. Dependencies: T05, T06, T11. Own: `internal/maintenance/discovery`, bot coordinator, finding endpoints.
- Build: import bot changes/CI/advisories, evidence fingerprints, group/supersession matching, no-bot Renovate onboarding proposal, persistent dismissal/snooze and change ownership.
- Accept: overlapping grouped upgrades and existing repairs never create duplicate bumps; bot/human branch modifications recognized; bot automerge authority conflict blocks Reforge control claim; snoozed findings reopen only by rule.

### T19 — Repair loop and validation recipes

- Owner: Astra for loop/validation trust; Luna for individual stack presets. Dependencies: T07, T12, T13, T14, T15, T18. Own: repair loop, validation-plan records, Go/JS/Python recipes.
- Build: baseline reproduction, plan, patch, pinned validation, bounded retries, trusted result artifacts, app-owned companion publication, independent task/change lifecycle. Add protected in-place bot repair only after exact-ref safety certified.
- Accept: real broken upgrade fixed end-to-end on fixture repositories; original failure reproducible; no passing status from deleted/disabled tests or changed script no-ops; cost/time/patch limits enforced; crash during publish reconciles one PR/MR.

### T20 — Findings, run evidence and change GUI

- Owner: Luna. Dependencies: T04, T17, T18, T19. Own: finding/run/change routes and evidence viewer.
- Build: attention queue, assignment/snooze/repair preview, stage timeline, source-linked baseline/candidate diff/artifacts, observed activity, live events, cancel/retry and current native gate detail.
- Accept: J02/J03; sanitised Markdown/logs/diffs; stale SHA evidence labelled; deep links preserve filters; no inaccessible artifact leak; no fake progress percentages.

### T21 — Merge eligibility and execution controller

- Owner: Astra. Dependencies: T06, T08, T09, T10, T19. Own: protection aggregation, merge controller, native queue reconciliation.
- Build: current H/B/T evidence, provider checks/reviews/CODEOWNERS and actor rules, exact-head guards, certified target enforcement, queue/train admission, execution-time policy checks and pause/cancellation races.
- Accept: J04/J06 plus policies acceptance matrix on three real provider instances; H/B change, spoof check, stale review, rule changes, external concurrent merge and lost response scenarios. No absolute cancellation promise after native admission.

### T22 — Existing CI/CD deployment orchestration

- Owner: Astra. Dependencies: T05, T06, T08, T09, T10, T21. Own: pipeline delivery adapters/controller.
- Build: allowlisted native workflows, trigger/observe modes, source/run/artifact correlation, native approval visibility, per-environment serialization, health verification and preauthorised recovery.
- Accept: native GitHub/GitLab approval paths certified; Gitea Actions used only within actual certified gate capabilities; external deployment tracking alone never supplies approval enforcement; ambiguous trigger result reconciled without duplicate deployment.

### T23 — Portable GitOps promotion path

- Owner: Astra. Dependencies: T06, T08, T09, T10, T21. Own: GitOps delivery adapter and provenance callback/read-only observer.
- Build: scoped delivery repo/manifest field/immutable digest configuration, deterministic patch PR/MR, protected merge gating, existing reconciler handoff, authenticated observed revision/health, preauthorised Git revert proposal.
- Accept: Gitea-backed promotion works via protected GitOps repo; source and delivery repo permissions enforced; incorrect digest/path/tenant rejected; no direct Kubernetes mutation; rollback recorded as distinct recoverable operation.

### T24 — Policy, deployment, usage and audit GUI

- Owner: Luna; Astra reviews authority presentation. Dependencies: T04, T06, T20, T21, T22, T23. Own: policy/deployment/usage/audit routes.
- Build: effective-policy editor/version simulation/rollout preview, inheritance explanations, merge gates, environment timeline, recovery preview, actual/estimated/unknown usage and audit export.
- Accept: J05/J07; “pipeline completed” distinguishable from “healthy”; subscription exhaustion never implies free fallback; disabled controls explain blockers; no GUI approval pretends to be a native review.

### T25 — Portfolio campaigns and scheduling

- Owner: Astra core, Luna GUI in a separate child ticket/branch. Dependencies: T05, T06, T18, T19, T21, T23, T24. Own: campaign service and subsequently campaign route.
- Build: pinned repo sets, canaries, observation windows, bounded expansion, stop thresholds, maintenance windows and tenant fairness.
- Accept: failed/unverified canary prevents expansion; changing a saved filter does not grow active campaign; budget exhaustion pauses correctly; 1,000-repo simulation does not starve another organisation.

### T26 — Self-hosted distribution and hosted operations

- Owner: Luna packaging under Astra review. Dependencies: T02, T03, T07, T16, T22, T23. Own: `deploy`, operator docs and upgrade tooling.
- Build: Compose edition, signed/pinned images, optional official-runtime packages with correct notices, outbound customer workers, hosted reference infrastructure/GitOps, backup/restore/retention/migrations, diagnostics.
- Accept: J09 from clean installation; restore encrypted credentials using backed-up keys; safe failed migration recovery; runner drain/rotation; same GUI/features in both editions; no Docker socket in API container.

### T27 — Isolation, concurrency and recovery qualification

- Owner: Astra. Dependencies: T19, T21, T22, T23, T25, T26. Own: cross-cutting scenario corpus and qualification report; fixes go back to owning tickets.
- Build/run: hostile repository suite, cross-tenant/team tests, DB/RLS under pooling, lease/queue/outbox crash injections, budget contention, forged/replayed webhooks, revoked identities and encrypted backup restore.
- Accept: J06–J10; zero policy bypasses; record every failing scenario and rerun only affected checks after fixes. Hosted untrusted execution stays disabled until isolation proof passes.

### T28 — Release qualification and operator handoff

- Owner: Astra coordinator with Luna browser/packaging assistance. Dependencies: all prior tickets. Own: release support matrix, acceptance report and remaining documentation.
- Run: documented product journeys, browser accessibility/empty/error states, real forge tests, model/agent support matrix, maintenance corpus, 10,000-repo deployment load, 100 concurrent runs, upgrade/restore, licence/dependency inventory.
- Accept: G5 checklist below, reproducible build artifacts, operator commands, documented limitations, known issues and support version ranges. No public release until owner invokes publication.

## G5 release checklist

- [ ] Apache-2.0 core and compatible dependency notices; source/build reproducible.
- [ ] Go+Gin backend; React/TS GUI; no hidden hosted-only maintenance gate.
- [ ] Hosted tenants and self-hosted edition work; customer-owned runner available.
- [ ] GitHub, GitLab and Gitea: connect, import, observe, publish, verify protection and merge on certified configurations.
- [ ] Renovate and Dependabot ownership/dedup/rebase behaviour tested; no redundant update solver.
- [ ] OpenAI, Claude, Google and compatible self-hosted API routes work.
- [ ] Eligible subscription bridge(s) documented/tested; ineligible routes disabled; no token harvesting or silent billed fallback.
- [ ] Policy-controlled merge plus existing CI/CD/GitOps deployments work, including portable Gitea delivery and native approval gates.
- [ ] GUI journeys J01–J10, accessibility and meaningful errors complete with real data.
- [ ] Isolation, race, crash, budget, retention, restore and load gates pass.
- [ ] Version-specific support/limitations and unresolved external entitlement limits accurately published.

## First implementation invocation

Start T01 only. Then integrate T02/T03/T04 before dispatching broader parallel work. A screenshot-only frontend or a server with mocked provider mutations does not satisfy G1/G2. Follow `agents.md` for worker prompts and integration discipline.
