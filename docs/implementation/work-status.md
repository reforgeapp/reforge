# Current work and review status

Updated 2026-09-23. This is the current index for open defects, remaining acceptance work
and certification. Older dated checkpoints remain evidence, not current ticket status.
G5 is **open**. The application is locally runnable; it is not release-certified.

## Review coverage

- GUI: all 13 main routes reviewed for cold/warm navigation and reload in two persisted
  organisations (86 states). Latest full browser run: 152 passed, six integration opt-ins
  skipped. [Data lifecycle report](data-lifecycle-review-2026-09-23.md).
- Visuals: earlier desktop/narrow and light/dark route review; subsequent shared-controls,
  switcher and Organisation corrections have their own dated evidence. Counts and axe
  results do not establish visual approval or complete every workflow.
- Backend: inherited unit, PostgreSQL race, recovery, restore, local OIDC, disposable Gitea
  and sandbox evidence exists. These suites were not rerun during the current layout fix.
- Current review reconciled the plan, handoff, support matrix and evidence; inspected the
  changed GUI and custom-command result handling. It was **not an exhaustive fresh audit
  of every backend, provider, security path and product acceptance criterion**. T28a closes
  that evidence gap. Do not read a previous “local complete” as “no remaining bugs”.

## Known defects

| Priority / ticket | State | Finding and completion criterion |
| --- | --- | --- |
| P1 / T31a | Open; reproduced locally | Custom-command executor accepts empty, progress-only and error-only exit-zero output as `completed_unverified`. Require a valid typed terminal result and reject missing, contradictory or failed protocol outcomes before extraction/validation. Independent frozen validation and publication authority still run; no protection bypass demonstrated. |
| P2 / T29s | Open; source-reviewed | Organisation displays green “Repository scope loaded” whenever `repos.error` is absent, including pending and partially paginated data. Represent loading, partial, complete and unavailable scope honestly; keep assignment controls and retry/load-more usable. |
| P2 / T29r | Fixed; browser-verified | Team creation button was centred against the label plus input. Input/button now align on desktop and narrow layouts. [Layout evidence](organisation-layout-review-2026-09-23.md). |
| P1 / T29q | Fixed; browser/backend-verified | Refresh/cached navigation emptied lists; incompatible team cache shapes crashed Organisation; filter loading stole focus. See linked lifecycle report. |

T31a proof: `.local/organisation-layout-2026-09-23/protocol-probe.jsonl`; isolated launcher
output probe, no real command/provider execution. Source: `internal/customcmd/executor.go`
and `internal/runnerclient/processor.go`. T29s source: `web/src/app/OrganisationPage.tsx`
(`TeamsSection` final status). Newly discovered issues belong here and in backlog with
reproduction, priority, owning ticket and closure evidence.

## Remaining local work

| Ticket | Next action | Required evidence / dependency |
| --- | --- | --- |
| T31a | Harden terminal-result protocol handling | Missing/duplicate/conflicting/error/malformed result, nonzero exit, timeout, cancellation and valid result regressions; verify processor does not advance rejected output |
| T29s | Correct repository-scope status in Organisation | Delayed, multi-page, complete and failed responses; no false success indicator |
| T28a | Complete release acceptance evidence map | Map V01–V29 and J01–J10 to exact builds, scenarios and artifacts; identify untested requirements. Recheck T31 extraction/validation and container-only clean install/upgrade/restart/restore. Signed local OIDC harness uses host Go and alone cannot establish the no-host-toolchain installation requirement |
| T28b | Execute actual scale target | 100 simultaneously executing runs plus 50 real browser sessions; latency, fairness, cancellation, restart and resource evidence. Existing 100 claims / 50 HTTP sessions are insufficient |
| T28c | Finish release dependency/notice review | Resolve each `review required` entry in `dependencies.md`, reconcile shipped images/packages and notices, rerun inventory for release build |
| T27 | Qualify hostile execution with enforced resource limits | Working cgroup-delegated runner host; run hostile/cross-tenant/budget/recovery corpus. Development rootless sandbox evidence does not certify hosted untrusted execution |

T28a–c are bounded children of T28; parent dependencies still apply. T28a can reconcile
evidence before external environments arrive, but cannot close missing external scenarios.
T28b/T27 require adequate local or authorised disposable infrastructure. No credentials
needed to fix T31a/T29s or review dependency/evidence records.

## External certification actions

| Tickets | Input required | Qualification action |
| --- | --- | --- |
| T08–T11, T21–T25 | Dedicated authorised GitHub/GHES and GitLab accounts/instances with claimed tier features; appropriate Gitea instance for wider version claims | Import, bot cooperation, publication/recovery, approvals, queues/trains, strict update, native deployment and campaign scenarios. Existing pinned disposable Gitea proof remains narrowly scoped |
| T12–T15 | BYO model accounts, explicit paid-test budgets and target self-hosted endpoint versions | Real tool loops, interruption, quota, usage and protocol-profile tests; local Ollama does not certify every compatible server |
| T16/T17/T31 | Official runtime/account, permitted entitlement/topology, pinned image and custody/approval evidence | Qualify Codex, Claude Code and `agy` separately, including cancellation, quotas and pre-effect control. Unknown/unsupported routes remain disabled |
| T26/T27 | Customer OIDC and authorised hosted/private-runner infrastructure | Identity, isolated execution, outbound/private routing, drain/restart and operator recovery. Kubernetes changes only through GitOps |
| T22/T23/T26 | Disposable native delivery environments and GitOps reconciler with health attribution | Preserve provider approvals and certify artifact/environment/health correlation; pipeline completion alone is not health |
| T28 | All required evidence above; owner release decision | Final support/version matrix and R01–R13/G5 review. Publication/external deployment requires separate authorisation |

## Ticket summary

| Tickets | Current state |
| --- | --- |
| T01–T06 | Locally complete; retained historical evidence |
| T07 | Local runner implemented; hosted isolation qualification open |
| T08–T15 | Local adapters implemented; external/version certification limited as above |
| T16/T17 | Backend-connected implementation present; qualification acceptance incomplete |
| T18–T25 | Locally complete; live/native-provider release evidence incomplete |
| T26/T27/T28 | Partial; local and external work listed above |
| T29 | Rebuild and T29a–r baseline locally verified; new T29s open |
| T30 | Docs engine/help locally verified; support-matrix corrections made; final release content follows T28a |
| T31 | Runtime/dispatch/extraction implemented; T31a defect open and release regression evidence pending |

Launch: existing app `http://127.0.0.1:8080`, docs `http://127.0.0.1:8082/docs/`.
Use [backlog](backlog.md) for acceptance/ownership, [validation](validation.md) for required
scenarios, and [progress](progress.md) for chronological evidence. The frontend large-chunk
build advisory remains a performance follow-up; no measured user-facing failure established.

The corrected support matrix passed the strict MkDocs container build and was verified
served by the existing local docs endpoint. Image `reforge-docs:gui-review`:
`sha256:1d227a08192da49737c62f972b6b0500e706aa4c01b4e993679380ef82fe29e6`.
Build log: `.local/organisation-layout-2026-09-23/docs-build.log`.
