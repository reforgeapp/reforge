# Worker execution runbook

Status: owner resumed GUI review/implementation, 2026-09-23. Read [PLAN.md](../../PLAN.md), [product-rebuild.md](product-rebuild.md) and [backlog.md](backlog.md). Earlier planning/usage pauses are superseded.

Current allocation override (owner, 2026-09-23): root/Astra owns architecture and final review. Prefer Claude CLI Opus 5.5 for bounded implementation, including sensitive work under coordinator review, until its usage window is exhausted; then use GPT-6 Luna or OpenCode DeepSeek 4.1-Flash for suitable slices. Maximum three workers total across providers, exclusive file ownership, no nested workers. CLI sessions use existing authorized account routes, not Reforge paid certification budgets. Older Luna-only allocation below is historical. Completion notifications or process completion preferred; avoid repeated model-driven status polling.

## Weekly usage stop rule

Owner instruction, 2026-09-24: after every worker completion (including failure), coordinator checks its current Codex weekly quota telemetry. Use the reported 10080-minute window; remaining percent is 100 minus used_percent. Below 40% remaining: stop expanding implementation scope, checkpoint, finish safe bounded wrap-up and bring up the local demo for review. Below 25% remaining: hard-stop active work jobs and preserve handoff; do not start replacement jobs. Do not infer remaining quota from token counts or model capacity errors. Record telemetry timestamp and remaining percentage. If unavailable/stale, report uncertainty and obtain a current reading before further dispatch.

## Initial invocation

Use this as the next implementation instruction after owner resumes work and reconciles `progress.md`:

```text
Implement remaining Reforge work in /home/mnorris/repos/reforge using PLAN.md, linked specifications and current progress. Preserve partial T25 and existing implementation; do not restart completed tickets. Follow explicit dependencies in docs/implementation/backlog.md, including T29–T31.
Root/Astra owns coordination, architecture and final review. Luna owns bounded implementation slices, including sensitive slices under Astra review.
Keep no more than three workers active alongside the coordinator. Assign disjoint files.
Preserve all R01–R13 requirements and both deployment editions. Track each ticket's status,
checks and remaining qualification evidence in docs/implementation/progress.md.
Use current supported dependency patches and official provider documentation when implementing.
Build and verify locally. Do not publish a repository/release, deploy infrastructure, merge
external PRs, consume live Reforge integration-test API budgets or change customer repositories without the
corresponding session authorisation. Fixture development does not require live credentials.
If a later integration gate needs accounts or infrastructure, continue independent tickets
and ask only for the specific missing input. Do not claim that fixtures certify live support.
Stop at a reviewable implementation with its acceptance report; publication is separate.
```

The initial T01 checkpoint freezes names, interfaces, enums, SQL migration ownership and API generation. T01 is implemented by Astra without parallel foundational edits. After review, T02 and T04 can proceed in parallel; T03 follows T02. The rest of the graph controls dispatch, not the illustrative wave order.

## Ownership and model allocation

| Role | Allocation | Responsibility |
| --- | --- | --- |
| Coordinator | Astra | Contracts, graph, shared files, integration, requirement coverage and review |
| Sensitive implementation | Luna, Astra review | Identity/tenancy, credentials, budgets, policy, sandbox, official-agent interception, protected merges and deployment |
| Bounded implementation | Luna, Astra review | One provider adapter slice, one GUI route/flow, one stack recipe or packaging slice |
| Independent review | Astra | Failure paths, trust boundaries, meaningful tests and acceptance evidence |
| Optional second opinion | Claude CLI, Opus5 only if actually available | Read-only review of a bounded diff/specification; no required dependency on this runtime/model |

Use `gpt-5.6-luna` for implementation workers; `gpt-6-astra` remains root review/coordinator only. A worker may not recursively delegate without coordinator allocation. Root plus three is the concurrency ceiling, not a target to fill when useful independent work is unavailable.

Split broad tickets into bounded child tasks before implementation. Each child has one observable outcome, exact files, inputs and acceptance checks. Keep the parent open until all children and integration checks pass. Prefer parallel Luna provider implementations only after Astra freezes the common interface; never let adapters invent different policy semantics.

## Worker brief template

```text
Ticket/child: Txx.y — <one outcome>
Model: Luna or Astra
Read: PLAN.md, AGENTS.md hierarchy, <relevant specifications>
Prerequisites: <completed ticket revisions and frozen interfaces>
Own: <explicit files/directories>; all other paths are read-only
Deliver: <behaviour and error paths>
Acceptance: <functional scenarios from parent ticket>
Shared changes needed: message coordinator; do not edit manifests/migrations/contracts
Constraints: preserve user changes; no real external mutations; no source comments or README
creation without authorisation; no trivial tests; do not weaken branch/tenant/budget controls
Return: files changed, behaviour, checks actually run and results, unresolved risks
```

Luna escalation: stop and report if the task requires a new shared interface, privilege change, ambiguous provider semantics or security architecture decision. After two unsuccessful attempts at the same substantive failure, send the reproducer and evidence to Astra; do not widen scope or delete a failing test to proceed.

## Integration procedure

1. Inspect the workspace and applicable instructions. Preserve unrelated changes. Create a local Git repository only as part of implementation if none exists; use `reforge` as provisional Go module until the owner chooses a remote.
2. Create `progress.md` with ticket, child, status, owner, files, dependencies, evidence and remaining gate. Statuses: `not started`, `in progress`, `review`, `local complete`, `certified`, `blocked`.
3. Coordinator allocates exclusive file ownership. After architecture freeze, coordinator may allocate specific shared files/contracts/migrations to Luna for implementation; root retains review/integration authority. Use isolated worktrees when helpful; workers cannot self-expand ownership. Coordinator controls dependency manifests/locks, shared interfaces, OpenAPI and migration numbering unless explicitly allocated.
4. Dispatch only ready children. UI can use typed fixtures before its backend exists, but the parent remains open until the real endpoint flow is integrated. Never advertise fixture progress as a working provider integration.
5. Worker runs the narrow acceptance checks and supplies its diff. Astra reads the complete change, checks error paths and confirms the tests exercise the stated risk. Run broader integration checks only when the change or unresolved concerns warrant them.
6. Integrate a small bounded change per task. Use accurate commit metadata if committing; do not rewrite authorship or fabricate development history. Update evidence and affected specifications before dispatching dependent work.
7. Resolve review findings in the owning child. A protected-merge, budget or tenant-isolation failure blocks that capability regardless of other successful tests.
8. At each gate, record what is locally verified versus certified against a real external system. Continue independent local work while external evidence is pending.

## Build and check contract for T01

T01 creates these commands; they do not exist in this planning directory yet:

| Command | Contract |
| --- | --- |
| `make dev` | Start local development with explicit fixture/auth mode and documented database prerequisite |
| `make generate` | Generate SQL/OpenAPI clients/types reproducibly; generated files have a single owner |
| `make build` | Build Go server/runner and bundled React GUI |
| `make check` | Formatting drift, Go vet, TypeScript checking and production frontend build |
| `make test` | Meaningful Go unit/service contract tests without external accounts |
| `make test-integration` | Disposable PostgreSQL/Gitea/runner fixtures and real persistence/concurrency behaviour |
| `make test-e2e` | Browser journeys against the running local application; fixture scenarios visibly isolated |
| `make qualify` | Explicit opt-in external certification; require supplied test accounts and bounded budgets |

Add Go race tests to concurrency/lease/policy changes and browser accessibility checks to changed flows. Use Vitest for frontend logic that warrants tests and Playwright for browser journeys; do not duplicate every Go policy test in the UI.

## Gate report and handoff

For G0–G5, record commit/build identity, deployment topology, runtime/provider versions, executed scenarios, pass/fail, evidence paths and limits. Complete the [validation matrix](validation.md). A ticket may be locally complete while its external certification is pending; it cannot satisfy the affected release gate until certified. This distinction prevents external-account delays from blocking unrelated development.

`Local complete` satisfies dependencies for local implementation against the reviewed contract. Only `certified` satisfies a live-provider release claim. Worker-model invocations already authorised for development are distinct from Reforge's future live integration-test API budgets.

Release handoff includes the reproducible install/build path, operator backup/restore and credential rotation instructions, support matrix, known limitations, maintenance corpus results and the R01–R13 checklist. Do not mark G5 complete if Gitea automation, hosted isolation or any required API family remains only mocked.

After all edits and Git actions, perform the applicable AGENTS.md cleanup of empty `.cc-writes`/`.claude` directories using `rmdir` only. Preserve the workspace root `.claude` and any non-empty directory.
