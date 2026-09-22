# Decisions and open items

Updated: 2026-09-22.

## 2026-09-22 — T29 review and reopen

- Direct review of `.local/visual/runners-1440.png`, `connections-1440.png` and `runs-1440.png` found styling/shell refinement but no proven structural operator-console rebuild. T29 is reopened in progress; backend and accessibility gains remain.
- T29a–f now govern IA/wireframes, shared list/detail layouts, Runners/Connections pilot, all remaining routes/auth/help, workflow/accessibility/responsive evidence and comparative visual review. Old screenshots remain reference evidence, not golden baselines.
- Visual acceptance requires same route/data/state before/after at 1440x900 and 390x844, route readiness, browser and built/served asset identity, cache reset, qualitative criteria and measured task paths. Heading/toolbar/axe counts and agent self-review are supporting checks only.
- Runners require searchable operational pool/runner inventory and API-backed heartbeat, capacity, trust and version details. Connections require Forge, Models & agents and Delivery tabs with proper profile management. Policies use named structured controls; raw JSON is advanced import/export only.

## 2026-09-21 — planning-only GUI/runtime rebuild

- T29 is one coordinated GUI rebuild/refactor across shell, navigation, design system, forms, tables and every route. Prior GUI completion acceptance is superseded; prototype is historical and has no visual authority.
- T30 is dedicated versioned/searchable MkDocs end-user, administrator and operator documentation, container-built/served and linked from in-app help.
- T31 defines versioned administrator-approved custom executable/argv profiles bound to image digest, protocol, policy and run. Claude Code, Codex and Google Antigravity CLI `agy` remain distinct. Runtime qualification requires entitlement, headless, container topology and pre-effect approval evidence; unverified means disabled, not successful.
- Planning pause records no implementation restart, release certification or provider entitlement. Existing progress evidence remains historical and current progress is authoritative for implementation state.

## Accepted from the owner

- 2026-09-21: T31 contract unfrozen. Current profile definitions are revisable design proposals. Preserve security requirements, existing versioned records and applied migrations; record replacement decisions and affected dependencies. This instruction alone does not launch implementation.
- Reforge; Apache-2.0 OSS; Go backend with Gin.
- GitHub, GitLab and Gitea required.
- BYO OpenAI, Claude, Google and self-hosted LLMs; supported subscription routes preferred when permitted.
- Native PR/MR workflow and branch-protection compliance; Renovate/Dependabot cooperation.
- Hosted multi-tenant service plus self-hosted edition.
- Primary users: engineering teams managing large repository portfolios.
- Policy-controlled merging and deployment automation included in v1.
- Deployments use existing CI/CD and GitOps workflows, including native approval gates.
- React/TypeScript selected for the frontend; owner accepts an industry-standard frontend.
- Implementation plan uses Luna/Astra workers; Claude CLI/Opus5 available if useful.

## Proposed defaults

| Decision | Default | Revisit trigger |
| --- | --- | --- |
| Name | Reforge | Owner-directed rename |
| Frontend | React + TypeScript + Vite; TanStack Query/Router; accessible components | Demonstrated implementation need |
| Backend | Go + Gin modular monolith plus separate Go runner binary | Independent scale/security boundary needs another service |
| Database | PostgreSQL 18, explicit SQL via pgx/sqlc, SQL migrations | Measured limitation; no embedded SQLite mode in v1 |
| Durable execution | PostgreSQL job leases + outbox + explicit states | Long-lived orchestration complexity warrants a workflow engine |
| Live GUI | Authenticated SSE plus ordinary JSON REST | Demonstrated need for bidirectional streaming |
| Isolation | Per-job sandbox, dedicated runner supervisor | Hosted sandbox gate fails; stop hosted execution until fixed |
| Authentication | OIDC + scoped organisation membership; bootstrap admin enrolment for self-hosted | Enterprise customer requires another auth standard |
| Deployment | Existing CI/CD and GitOps workflows, confirmed | Owner selects a direct hosting integration |
| Licensing | Apache-2.0, confirmed | Owner-directed change before distribution |
| Business layer | Invite-based hosted pilot; usage metering, no payment integration | Commercial launch |
| Repository remote | Pending; do not create/publish one during planning | Implementation owner chooses organisation/module path |

## Release gates, not unanswered product questions

- Certify each supported forge/server version against a real test instance and record missing capabilities. Public documentation alone is insufficient for an automated-merge claim.
- Confirm subscription eligibility for each adapter, account type and hosted/customer-owned topology. Default to BYO API if the official route cannot be established; do not extract or impersonate OAuth credentials.
- Select and prove one hosted isolation runtime, image policy and runner host boundary before untrusted tenant execution.
- Exercise native deployment approvals and correlation on each forge; unsupported approval APIs cannot be bypassed by application policy.
- Owner supplies live test accounts, model budgets, organisation/SSO configuration and hosting destination at the implementation stages that need them. These do not block local fixtures, GUI, contracts or architecture.

## Decision procedure

Record changes here with date and affected requirement/task IDs. Preserve hard requirements; deferring an item needed by G5 moves the release gate rather than silently reducing scope. Treat research documents as evidence, and product/contracts/policies as the selected design. If evidence contradicts a selected design, correct the design and dependent tickets before implementation.

## 2026-09-20 — T01 implementation freeze

- Module `reforge`; Go 1.27.1, Gin 1.12.0, pgx 5.11.0, PostgreSQL 18.6; current patches verified against official release sources. React 19.3.0/Vite 8.3.0; TypeScript 5.9.3 selected because openapi-typescript 7.13.0 requires the supported 5.x line. No peer-dependency override.
- Coordinator owns shared interfaces, OpenAPI/SQL generation and migration ordering. `contracts-freeze.md` records concrete Go boundaries. Cancellation of stateless direct inference is contextual; stateful official runtimes expose explicit cancellation.
- Runtime database role must lack direct or inherited schema/table/database ownership, bypass, role-management and database-creation privileges. Migration command uses separate credentials and checksum-verified transactional SQL.
- Explicit development fixture mode requires numeric loopback origins/listening addresses and self-hosted edition; production has no silent fixture fallback.
- Local Docker daemon unavailable. Built checksum-verified PostgreSQL 18.6 source under `/tmp/reforge-postgres` for actual database tests, using isolated parser build tools. This does not certify the later Compose distribution or hosted sandbox.
- Source review: [Go releases](https://go.dev/dl/), [PostgreSQL support](https://www.postgresql.org/support/versioning/), [row security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html), [Gin](https://gin-gonic.com/en/docs/).


## 2026-09-20 — Private operations and inventory (T07/T11)

- Private endpoints use outbound, fixed-operation supervisor grants. The API never receives a dialable private HTTP client. Readiness precedes fresh tenant/runner/fence authorization; result completion does not acquire a second DB connection. The initial coordinator is process-local and requires endpoint affinity; durable effect intents and usage records remain in PostgreSQL.
- Empty runner pools support onboarding/private metadata but carry no repository job authority. Paid inference and writes require their own durable budget/intent admission before private dispatch; generic proxy operations are absent.
- Inventory/import runs asynchronously with bounded provider pages and fresh connection versions. Only a complete authoritative scan can remove repository access. Provider native IDs preserve identity through renames. Webhook payloads prompt authoritative refresh, never grant execution authority.
- Background tenant scheduling reads a UUID-only catalog; an invoker trigger registers new organisations. Every operational query still uses tenant context/RLS under the restricted runtime role. No cross-tenant payload index or RLS bypass is added.

## Repair recipe v3 — 2026-09-21

Recipe turn ceiling is 16. The eight-turn live dependency fixture exhausted its allowance after reading source/tests/both dependency implementations and making two patches; original-target compatibility was still unresolved. This is a versioned recipe change, not a bypass: frozen plans retain their own ceilings, every model turn requires a fresh budget reservation, and organisation/route/time limits still stop execution. A twelve-turn follow-up reached the correct compatibility diagnosis but exhausted before applying it; sixteen turns passed the real controller/runner/upgrade fixture in153.769s. Local fixture request budgets explicitly allow 16; no paid API budget is introduced.

Validation evidence is reused only while the staged patch and immutable source/target plan are unchanged. Read-only tool turns no longer rerun identical checks or repeat failure logs. Any changed patch invalidates candidate and target results. Explicit `run_checks` still executes. Native candidate validation remains a separate mandatory boundary.

### GitLab train release profile — 2026-09-21

Use the native protected blocking manual job on the exact train pipeline. Source-head statuses do not release train execution. Initial supported profile requires standalone immutable CI configuration, one exact H/T candidate, a dedicated protected gate environment and the operational user as its sole permitted releaser. Persist release intent before native play; never retry uncertain play. Unsupported/multi-predecessor configurations remain actionable blocked states pending qualification. Canonical native merge alone concludes the operation. No new deployment engine or Kubernetes mutation is introduced by this gate.

## 2026-09-21 — T23 GitOps authority and evidence

- GitOps changes one configured YAML/JSON scalar to a registry-qualified immutable digest. RFC6901 selects the field; AST validation plus semantic comparison preserves all unrelated fields. Input size, token count and nesting are bounded before parsing. Unsupported constructs produce a blocked action.
- Source and delivery are independent repository scopes. Both policies, connection versions and original requester authority remain live guards. The protected merge controller recognises recorded GitOps branches and rejects unrecorded owned prefixes; source revocation also applies to direct merge endpoints and queued execution.
- Deterministic candidate proof supplies local manifest validation to the existing merge controller. Native checks, approvals, strict target/head enforcement and non-bypass actor requirements remain mandatory. Preview displays policy/native blockers; no UI action supplies a native review.
- Stage and publication use separate durable one-shot intents. Persistence checks dispatch identity so a concurrent loser cannot overwrite an active operation. Uncertainty is reconciled by canonical commit/tree/PR identity. Cancellation after uncertain publication retains environment ownership until the native outcome is known.
- Health binds source, digest and actual delivery merge revision to a configured Ed25519 observer and elapsed freshness window. Merge alone is unverified. Recovery is a distinct protected proposal to a previously healthy artifact on the same manifest target, followed by new health observations. No direct Kubernetes mutation or Git history rewrite.
