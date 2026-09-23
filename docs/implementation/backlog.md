# Implementation backlog

## Current open work — 2026-09-23

[Current work status](work-status.md) is the authoritative open-defect and acceptance index.
The dated sections below preserve prior evidence; they do not override newer findings.
T29r and T29s are locally complete. T31a remains open. T28a–c, hosted isolation
and external certification remain required; G5 has not passed.

### T29r — Align Organisation team creation

- Status: **local complete, 2026-09-23**. Depends T29q; scoped GUI change only.
- Own: Luna `OrganisationPage.tsx` and scoped `app.css`; Astra review.
- Accept: label/input and Create team button align without desktop wrapping; keyboard
  and 1440/390/320px in both themes pass, existing team/membership regressions retained.
- Evidence: [layout review](organisation-layout-review-2026-09-23.md).

### T29s — Truthful repository scope status

- Status: **local complete, 2026-09-23**. Depends T29q; scoped Organisation GUI change.
- Result: pending, partial, refreshing, complete and unavailable states reflect repository
  query state; failed pagination exposes retry while retaining loaded assignment controls.
- Evidence: [T29s review](t29s-review-2026-09-23.md).

### T31a — Validate custom-command terminal results

- Status: **open, P1; locally reproduced**. Depends T31. Astra protocol design/review,
  Luna implementation with explicit ownership of executor and affected processor tests.
- Cause: `Executor.classify` accepts empty, progress-only and error-only exit-zero streams
  as `completed_unverified`, contrary to the required typed terminal result.
- Fix: specify/enforce terminal outcome and ordering; reject missing/duplicate/conflicting
  results and error outcomes before advancing extraction/validation. Preserve independent
  frozen validation, usage accounting, cancellation, fencing and publication authority.
- Accept: valid success advances only to validation; missing/malformed/error/nonzero/
  truncated/timeout/cancelled outcomes cannot advance. Processor regression plus existing
  real-container protocol and patch validation checks. No live API budget needed.
- Evidence: `.local/organisation-layout-2026-09-23/protocol-probe.jsonl`.

### T28a — Reconcile release acceptance evidence

- Status: **open**. Child of T28; retain its full dependency graph. Astra reviews, Luna
  owns bounded evidence/packaging slices. Independent evidence audit can begin now.
- Map V01–V29/J01–J10/R01–R13 to implementation and exact observed builds/artifacts.
  Explicitly mark not run, inherited evidence, fixture-only and externally certified.
- Verify T31 extraction/validation after T31a and a fresh container-only clean install,
  upgrade, restart and encrypted restore without host language toolchains or `/tmp` tools.
- Accept: every requirement has proof or a specific owning open task; no stale completion
  claims; no G5 claim while mandatory evidence is absent.

### T28b — Real execution and browser load

- Status: **open**. Child of T28; T07/T25/T26/T27 and adequate isolated runner/browser
  resources required for execution. Luna harness, Astra isolation/budget review.
- Accept: 100 concurrent executing runs and 50 real browser sessions under documented
  latency/fairness/recovery criteria. Claims and HTTP sessions do not substitute.

### T28c — Finish dependency and distribution inventory

- Status: **open**. Child of T28. Luna inventory, Astra release review.
- Resolve `review required` dependency entries against actual shipped versions; include
  container/runtime assets and required notices. Record evidence, not inferred licence labels.
- Accept: reproducible release inventory, notices and explicit decisions; separate
  publication authorisation remains necessary.

## Owner resumed GUI work — 2026-09-23

Latest owner instruction supersedes the pause below. Review Deepseek T29k/l/m, repair Findings/theme issues, rebuild Policies, then finish outstanding GUI acceptance. Astra coordinates/reviews; Luna implements. See current `progress.md`; earlier checks remain historical.

### T29q — Reliable list data through navigation and refresh

- Status: **local complete, 2026-09-23**. [Review and evidence](data-lifecycle-review-2026-09-23.md). Depends T29h/i and current GUI; no new API contract.
- Cause: local list copies cleared while structurally shared query responses retain identity. Confirmed Connections: 33 server records become zero displayed rows after Refresh or cached-tab selection.
- Own: Luna admin slice (Connections, Runners, list query builders, regressions); Luna work-list slice (Repositories, Findings, Runs, regressions); Luna Organisation cache-shape slice. Astra reproduces/reviews/integrates.
- Build: query-owned paginated lists, tenant/filter-specific keys, no secondary resettable row copies. Refresh and mutations retain/replace rows from authoritative responses. Separate finite/infinite cache shapes; keep filters mounted during loading/errors. Preserve cursor traversal, back/deep links, saved views, cancellation, errors and tenant isolation.
- Accept: unchanged-response refresh, same/cached tabs and filters, repeated saved-view load, multi-page refresh/removal, genuine empty responses, retry and late-response isolation. Tests fail against old served bundle before repair; connected backend navigation/refresh plus integrated browser checks pass afterward. Review all routes for the same pattern; document remaining limitations precisely.

### T29p — Shared controls, surfaces and search

- Status: **local complete, 2026-09-23**. [Review and evidence](shared-controls-review-2026-09-23.md). Depends T29b/d/j/o; no new backend contract.
- Own: Luna CSS/tokens slice and separate Luna AppShell/search-tests slice; Astra reviews/integrates.
- Build: replace redundant nested panel frames with spacing, quiet surfaces and meaningful dividers; consistent inputs/selects/buttons; one focus indicator per compound control. Keep labelled fields and meaningful status/security boundaries. Search uses existing scoped repository API flow, proper submit control and disabled native form-history suggestions; no fake autocomplete. Mobile search remains usable without compressing input beside every header action.
- Accept: before/after actual route review in both themes; focused/open/error/blocked states; keyboard and tenant-scoped search, submit/clear, URL filters/back; responsive header/menu geometry at 320/390px; no new accessibility/overflow defects; matching served/container assets and truthful evidence.

### T29j follow-up — Integrated organisation switcher

- Status: **local complete, 2026-09-23** after owner screenshot feedback. [Follow-up review](organisation-switcher-review-2026-09-23.md): 17 passing shell/theme tests and actual open-menu checks in both themes at desktop/390px.
- Own: AppShell, scoped switcher styles and shell regression tests; Luna implements, Astra reviews.
- Accept: borderless inline header trigger; dropdown aligned to trigger and contained on narrow screens; compact names with current marker, Paused only where applicable; no horizontal scrollbar; independently scrolling list and always-visible More action. Preserve keyboard/focus, modal and tenant cache behavior. Verify many/long names, both themes, desktop and 390px against real session data and fixtures.

### T29o — Consistent Policies workspace

- Status: **local complete, 2026-09-23**. See [review evidence](gui-feedback-review-2026-09-23.md).

- Owner: Luna; Astra reviews policy-state/security preservation. Depends T29b/d; coordinate T29g after layout review.
- Build: compact repository/scope toolbar, concise effective policy, connected editor/version history, shared reason/save controls, deliberate simulation surface. Established console theme; advanced evidence collapsed. Dedicated responsive layout keeps editor visible at 390px.
- Accept: owner/admin/read-only permissions, CSRF, immutable versions, presets, zero limits, required/default/forbidden rules, dirty-draft simulation invalidation, activation hash/CAS preserved. Keyboard and both-theme desktop/narrow review; real backend save/reload/simulation; no repeated explanations or raw UUID dominance.

## Historical usage pause — 2026-09-22 (superseded)

Stop further implementation after the closing worker batch. Outstanding T29g–n tickets, exclusive file ownership, acceptance checks and Copilot/Luna suitability are in [the Copilot handoff](copilot-handoff-2026-09-22.md). Current pause overrides earlier autonomous continuation instructions. Broader release tickets remain open.

## Historical review authority — 2026-09-22 (superseded)

GUI rebuild locally verified. T29 acceptance remains open for documented policy workflow gaps: automatic stored-evidence/portfolio simulation. The delivered GUI is a restrained operator console with a white workspace, light slate navigation, blue 14px controls, flat inventory surfaces and functioning tabs/workspaces. See [GUI policy requirements](../design/gui.md). Historical “complete” rows remain evidence history. No G5, human approval or external certification is implied.

Current checks: Go tests and `make check` pass; browser final `.local/rebuild-browser-final.json` records 108 passed, 0 failed, 9 opt-in skipped; policy/insights `.local/policy-final.json`, persisted controls `.local/live-controls.json`, and fresh route/detail captures under `.local/rebuild-final/` pass. The historical full-suite `83 passed, 24 failed, 9 skipped` result remains comparison history.

Updated: 2026-09-22. Detailed planning history remains below; current implementation status is recorded above. Do not erase backend gains or restart unrelated implementation.

## Working rules

- R01–R13 in `../../PLAN.md` are release requirements. No worker may silently defer one.
- Root/coordinator owns shared contracts, module/package manifests, migrations ordering and integration branches.
- Astra owns ambiguous/security-sensitive architecture, tenancy, execution, protection/merge and deployment work. Luna owns bounded adapter/UI/recipe work after contracts are frozen. Every Luna change receives root/Astra review.
- Current allocation override: Luna owns implementation slices, including sensitive slices under Astra review. Root/Astra owns architecture, integration and final review, and allocates exclusive Luna ownership for shared files/contracts/migrations only after architecture freezes. Workers cannot self-expand ownership; historical labels do not authorize unallocated edits.
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
| W7 | T25 campaigns, T31 custom command runtime, T29 GUI design child | Design and runtime contracts proceed after their explicit prerequisites |
| W8 | T26 distribution/operations, T27 isolation/recovery qualification, T29 GUI route completion, T30 MkDocs documentation | Integrate bounded work; T28 remains final release qualification |
| W9 | T28 release qualification and operator handoff | Final integrated qualification |

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
- Build: shell/navigation, organisation scope, URL filters, query client, status/gate components, accessible tables/dialogs, standard empty/loading/error/stale states. Use `docs/implementation/product-rebuild.md` and GUI spec as authority; prototype is historical only.
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
- Build: official bridges remain T16-owned. Add Google Antigravity CLI `agy` as distinct qualification target; it is not Gemini CLI. T31 custom profiles reuse this lifecycle contract but do not replace official bridge ownership.
- Accept: actual runtime version tested for auth/logout/revocation/quota, approvals before effects, credential custody and shell isolation. An approval request is not simulated from a post-execution event. Subscription route status/billing visible; no API fallback without explicit permission/budget.
- Gate: eligible documented subscription/workspace route preferred when available; uncertain route disabled with reason. Direct API support for all required model families remains mandatory. No unsupported promise of universal subscription access.

### T17 — Portfolio and connection GUI

- Owner: Luna. Dependencies: T02, T03, T04, T11; model forms depend on T12–T16 capabilities. Own: repository/onboarding/connection/runner list routes.
- Build: import wizard for all forges, repository detail/baseline, scoped filters/saved views, connection health/model billing-route/capabilities, write-only rotation and enrolment UX.
- Build: GUI must expose actual SaaS and OSS/self-hosted qualification flows for auth/bootstrap, runner placement, model/agent capability probes, entitlement/quota status and revocation. Unsupported or unverified runtime/topology stays disabled with actionable remediation; fixture-only screens cannot close T17.
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

- Owner: Luna packaging under Astra review. Dependencies: T02, T03, T07, T16, T22, T23, T31. Own: `deploy`, operator docs and upgrade tooling.
- Build: Compose edition, signed/pinned images, optional official-runtime packages with correct notices, outbound customer workers, hosted reference infrastructure/GitOps, backup/restore/retention/migrations, diagnostics. Containerise server/GUI, controllers, migrator, runner, agent profiles, validation images, docs and local dependencies. Clean install needs no host Go/Node/Python or repository-specific `/tmp` paths. Document chosen runtime host capabilities; Docker is not tenant isolation. No API/task container gets a Docker socket; credentials never bake into images.
- Accept: J09 from clean installation; restore encrypted credentials using backed-up keys; safe failed migration recovery; runner drain/rotation; same GUI/features in both editions; no Docker socket in API container.

### T27 — Isolation, concurrency and recovery qualification

- Owner: Astra. Dependencies: T19, T21, T22, T23, T25, T26, T29, T30, T31. Own: cross-cutting scenario corpus and qualification report; fixes go back to owning tickets.
- Build/run: hostile repository suite, cross-tenant/team tests, DB/RLS under pooling, lease/queue/outbox crash injections, budget contention, forged/replayed webhooks, revoked identities and encrypted backup restore.
- Build/run: qualify all containerised components and clean-install/upgrade paths, chosen sandbox host prerequisites, custom profile protocol and docs/help route integrity alongside hostile repository and recovery suites. Docker socket and baked-credential checks are mandatory.
- Accept: J06–J10; zero policy bypasses; record every failing scenario and rerun only affected checks after fixes. Hosted untrusted execution stays disabled until isolation proof passes.

### T28 — Release qualification and operator handoff

- Owner: Astra coordinator with Luna browser/packaging assistance. Dependencies: T01,T02,T03,T04,T05,T06,T07,T08,T09,T10,T11,T12,T13,T14,T15,T16,T17,T18,T19,T20,T21,T22,T23,T24,T25,T26,T27,T29,T30,T31. Explicit acyclic list; no shorthand “all prior tickets”. Own: release support matrix, acceptance report and remaining documentation.
- Run: documented product journeys, browser accessibility/empty/error states, real forge tests, model/agent support matrix, maintenance corpus, 10,000-repo deployment load, 100 concurrent runs, upgrade/restore, licence/dependency inventory.
- Accept: G5 checklist below, reproducible build artifacts, operator commands, documented limitations, known issues and support version ranges. No public release until owner invokes publication.

### T29 — Full GUI rebuild and visual regression

- Status: **rebuild locally verified; T29s follow-up locally complete, 2026-09-23**. T29a–r baseline retained. Latest full-suite evidence is [T29q](data-lifecycle-review-2026-09-23.md), with focused [T29r](organisation-layout-review-2026-09-23.md) and [T29s](t29s-review-2026-09-23.md) afterward. See [current work](work-status.md); G5/provider/release qualification remains separate.
- Owner: Luna; Astra owns architecture and authorisation review. Dependencies: T04,T16,T17,T20,T24,T25,T31. T29 does not depend on T26,T28 or T30; T28 checks integrated docs/help links. Parent dependency list remains explicit and acyclic.
- Build: structural operator-console rebuild across shell/navigation/design system/forms/tables/work surfaces and every route. Preserve Go/React APIs, security and existing backend gains. Use Rundeck/AWX as interaction references only. No framework rewrite or line-count quota.
- T29a — IA and wireframes: depends T04. Record route hierarchy, connected list/detail interaction, Runners and Connections pilots, responsive states and qualitative acceptance against Rundeck/AWX research. No user approval gate.
- T29b — shell/resource layouts: depends T29a,T04. Implement shared hierarchy and resource inventory/detail patterns; remove repeated intro/card clutter; retain labels, errors, security and unknown states.
- T29c — Runners and Connections vertical slice: depends T29b,T17,T31. Complete real persisted list/detail, search/filter/pagination, health/heartbeat/capacity/trust/version fields, named repository selectors, guided enrol/create/test/rotate/revoke flows and safe progress/handoff states. Connections tabs: Forges, Models & agents, Delivery.
- T29d — remaining routes and auth/help: depends T29b,T20,T24,T25,T31. Propagate connected list/detail/evidence patterns to Overview, Repositories, Findings, Runs, Changes, Deployments, Campaigns, Policies, Usage, Audit, Organisation and auth/help. Policies use named structured controls; raw JSON only advanced export/import.
- T29e — workflow, accessibility and responsive states: depends T29c,T29d. Exercise representative operator tasks with persisted records at desktop and 390px; cover keyboard, loading, empty, error, blocked and stale states; measure completion/click path.
- T29f — comparative visual review: depends T29e. Capture same viewport/data/state before/after at 1440x900 and 390x844, record built/served asset identity and cache reset, review qualitative criteria and findings. Selector/heading/axe counts are supporting checks only; agent review is not human sign-off.
- Accept: all children complete; route coverage and operation/help matrix complete; no full-page UUID fixture or fake production fallback baseline; SaaS/OSS workflows use real records; no SQL/curl/raw JSON primary journey. Preserve historical screenshots as reference only.
- Existing T17/T20/T24/T25 backend and functional checks remain retained evidence; their GUI acceptance is superseded by T29’s reopened children. T30 revalidation is limited to help/content affected by route changes; do not redo the MkDocs engine without a regression.

### T30 — Versioned MkDocs end-user, administrator and operator documentation

- Owner: Luna; Astra reviews security and operational claims. Dependencies: T16,T19,T21,T22,T23,T26,T29,T31. T30 may draft earlier; release acceptance waits on these flows.
- Build: versioned/searchable MkDocs site, container build and served docs, in-app deep links/help drawer; document clean Compose install, hosted GitOps, provider auth, native approvals, runner host prerequisites, secret custody, unsupported capabilities and recovery. Reference https://www.mkdocs.org/.
- Accept: docs container builds/serves from clean environment; every route family links help; SaaS and OSS workflows executable from docs; no entitlement or certification claim lacks evidence.

### T31 — Administrator-approved custom command runtime profiles

- Contract: **unfrozen** by owner instruction, 2026-09-21. Revise schema, interfaces, protocol and execution design as needed; reconcile dependent T16/T26/T29/T30 contracts and acceptance before integration. This supersedes the contract-freeze prerequisite for T31 design work, not its security requirements or exclusive file ownership.
- Owner: Astra for security/runtime contracts; Luna may implement bounded forms. Dependencies: T03,T05,T06,T07. Profile identity is versioned and binds executable, fixed argv, container image digest, protocol version, input/events/output/cancel/exit/usage semantics, approval record and policy/run references.
- Build: tenant cannot submit arbitrary shell or executable path; runner executes only approved image/profile with typed argv/input and no shell interpolation; enforce wall-clock/output/turn/concurrency budgets; isolated secret custody; protocol records malformed events, unknown usage, cancellation, timeout and nonzero exit; audit approval/revocation; visible capability state.
- Accept: real local container test covers input/output, malformed events, nonzero exit, timeout, cancellation, revocation and secret isolation. Exit 0 never means validated repair. Entitlement, headless mode, container topology, approval interception and exact runtime/version each evidenced before enablement; unsupported/unverified combinations visibly disabled with actionable reason; run/profile/policy/image digest immutable and revocation fences future effects.

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
- [ ] T29 route coverage passes one-title/one-toolbar/dense-work-surface gate with restrained interface copy, no generic explanations and no copied assets; all SaaS and OSS workflows use real records.
- [ ] T30 MkDocs end-user/admin/operator docs are versioned/searchable, container-built/served and linked from in-app help.
- [ ] T31 custom profiles are admin-approved, digest-pinned, protocol-qualified and visibly disabled until entitlement/headless/container/approval gates pass.
- [ ] All components containerised; clean install needs no host Go/Node/Python or repository `/tmp`; chosen runtime host prerequisites documented; Docker is not tenant isolation.
- [ ] Isolation, race, crash, budget, retention, restore and load gates pass.
- [ ] Version-specific support/limitations and unresolved external entitlement limits accurately published.

## First implementation invocation

On resume, reconcile `progress.md`, preserve partial T25 and dispatch only ready children. T29–T31 follow explicit dependencies; a screenshot-only frontend or server with mocked provider mutations does not satisfy G1/G2. Follow `agents.md` for worker prompts and integration discipline.
