# Current work and review status

## Resumed coordinator checkpoint — 2026-09-23

User resumed implementation after reboot and authorized Opus-first CLI workers, falling back to lighter models when the Claude usage window is exhausted. Maximum three workers total; exclusive file ownership; no nested workers. Coordinator owns review/integration, not product implementation. Claude auth reports first-party `claude.ai` Pro subscription with no ANTHROPIC_API_KEY; all three initial workers resolved `opus` to `claude-opus-5-5`. No Reforge paid provider certification is authorized by worker-model access.

Baseline HEAD `cd2b94a`. Latest full browser evidence is **153 passed, 27 failed, 15 skipped** at source `35b7545`, superseding earlier passing-suite summaries below. See [current browser report](t28a-current-browser-review.md). Invitation backend committed `20582c1`; pending GUI and Audit/test changes survived reboot and remain under review. Preserve unrelated `copilot-handoff-2026-09-22.md` edits. G5 remains open.

| Worker | Exclusive scope | State / next check |
| --- | --- | --- |
| Opus identity | Organisation invitation GUI/API, invite landing, route registration; AppShell invitation bypass only; associated styles/tests | Finish signed local IdP new-member callback, replay/revoke and responsive GUI verification |
| Opus audit_policy | Audit/Policies page components and route styles; insights, policy-impact and Audit close-panel test | Review pending fixes, reproduce failures, focused tests and accessible light/dark desktop/mobile captures |
| Opus lifecycle | Connections/Changes components and route styles; lifecycle/team-cache/shell/theme/workspace/changes tests | Separate real data/focus bugs from stale fixtures; fix with meaningful regression checks |

Worker briefs/session logs: `/tmp/reforge-opus-resume/`; durable acceptance artifacts belong under `.local/opus-resume/`. Shared fixture browser server: `127.0.0.1:5173`. Docker init script failed with ulimit error; directly launched local dockerd and verified daemon responds. Worker-owned disposable services may now be recreated; prior process IDs are not reused. Persistent goal tool still reports paused and exposes no resume operation; user resume authorization is recorded here while implementation proceeds.

Reviewed integration: `8846d9d` closes the Audit/Policies regression slice (36 focused checks; 8 light/dark desktop/mobile accessibility probes). The second Opus worker now owns only `Workspace.tsx`, `styles/app.css` and a targeted shared-focus test. Invitation worker additionally owns redemption HTTP handler/tests and its contract to fix CSP-safe JSON authorization navigation; global CSP remains unchanged. Lifecycle worker scope unchanged.

Coordinator restricted-role auth/HTTP race tests passed against a fresh migrated database. The earlier run on the integration-populated database failed its single-fixture-organisation assumption; both logs retained under `.local/opus-resume/integration/`. This does not turn the failing reused-fixture run into a pass.

Remaining after this batch: review/integrate each diff, run exact-current browser/Go integration gates, resolve any remaining local failures, current container/source/notice checks, positive protected-merge acceptance if local prerequisites can be provided, and external certification inventory. Existing cgroup/hosted/provider/legal/human acceptance gaps are not waived.


Updated 2026-09-23 against HEAD `35b7545`, including reviewer acceptance `7175484`, connected Identity GUI `35b7545`, OIDC runtime `08669b4`, and Alpine provenance review `af230bd`. Existing-member OIDC flow is locally verified; first-user onboarding remains in progress; positive protected merge, current full suite/image and external certification remain open. This index separates local verification from remaining local acceptance and external certification. The latest full browser suite predates current GUI commits.
G5 is **open**. Application is locally runnable; it is not release-certified.

## Review coverage

- GUI: 13 routes across cold/warm navigation and reload in two persisted organisations (86 states). Latest full Go-served Playwright run at `cce6879`: 168 discovered, 154 passed, 14 opt-in skips, 0 failed/flaky. It predates GUI changes after `cce6879`, including the connected Identity workflow; focused live Identity acceptance passed at `08669b4`. Focused current GUI checks cover Policies, Audit, Connections, Organisation Teams/Members, Repository Sync picker and the Overview sync warning; connected backend checks cover policy and team/member persistence. [Full suite review](t29-final-browser-review.md), [Policies](t29-policies-connected-review.md), [Audit](t29-audit-rebuild-review.md), [Connections](t29-connections-rebuild-review.md), [Organisation](t29-organisation-rebuild-review.md), [Overview sync attention](t29-overview-sync-attention-review.md).
- Scale: 10,000 fixture repositories across ten orgs; asynchronous scan/import, page/search, cross-org denial recorded. Separately, 50 browser sessions completed 250 list and 250 detail reads. Neither exercises 100 simultaneous runs or hosted scheduler fairness. [Scale review](t28b-repository-scale-review.md), [browser review](t28b-browser-review.md).
- Local security/operation: endpoint/DNS fixture corpus, process-killed workflow/runner lease recovery, campaign missing-health/replay/fairness and bounded recipe/tamper corpus now have dated reports. Four real local Ollama repair attempts produced no valid candidate. These results do not establish hosted cgroup enforcement or provider certification.
- Clean local amd64 control/runner/docs images were built at detached `cce6879`; the control frontend matches the web stage and all three images include the generated notices. These checks do not prove current `35b7545` source/image identity, arm64, full reproducibility or legal attribution. Rebuild/review current source. Human visual baseline approval remains open.
- A Go race suite passed earlier without PostgreSQL integration. The latest guarded Go/PostgreSQL integration run **passed**: `.local/t28a-final-integration.log` ended `ok reforge/test/integration 183.115s` after migrations applied at 16:19 AEST. The earlier ENOSPC attempt in `.local/postgres-review.log` is superseded, not the latest result.
- Repository Sync picker fix at `208eef6` is connected-verified on disposable Gitea/Go/PostgreSQL. Latest one-test Playwright run built server from working tree at HEAD `af230bd` and independently verified the healthy connection after reload and on a separate fresh page; import, tenant denial, stale-gate exact 409, native reviewer request, zero pre-approval and one current-head approval also passed. Unqualified merge returned exact 409 because policy/qualification evidence is absent. Do not claim an authorized merge. Organisation OIDC runtime/session binding is committed as `08669b4`; connected save/probe/activate/reload/sign-in/disable acceptance passed against it, with captures and API evidence in [Identity GUI review](t29-identity-active-gui-review.md). First-time IdP invitation/onboarding implementation remains in progress; customer IdP interoperability is external. Restricted-role PG/race results remain worker-reported without retained output. See [picker review](t29-repository-sync-picker-review.md), [reviewer evidence](t28a-reviewer-journey-review.md) and [Identity contract](t29-identity-contract.md).

## Known defects

| Priority / ticket | State | Finding and completion criterion |
| --- | --- | --- |
| P1 / T31a | Locally fixed; runtime qualification open | Exit-zero custom-command output advances only with exactly one final `result` carrying `data.outcome: "success"`; missing, malformed, unknown, duplicate, misplaced, failed and error-event outcomes cannot advance extraction. External runtime/account/topology qualification remains open. |
| P2 / T29r | Fixed; browser-verified | Team creation button was centred against the label plus input. Input/button now align on desktop and narrow layouts. [Layout evidence](organisation-layout-review-2026-09-23.md). |
| P2 / T29t | Locally fixed; browser-verified | At 390px, open navigation had no scrim/outside dismissal and Tab reached controls behind it. Drawer now traps focus, inerts background, closes on scrim/Escape/route selection and restores focus. [Review](t29t-mobile-navigation-review.md). |
| P1 / T29q | Fixed; browser/backend-verified | Refresh/cached navigation emptied lists; incompatible team cache shapes crashed Organisation; filter loading stole focus. See linked lifecycle report. |
| P1 / T29u | Connected negative-path acceptance complete (`7175484`); positive authorized merge open | Picker refreshed and selected newest healthy Gitea connection. Reload and separate fresh-page picker paths both showed latest healthy connection. Native change, stale-head exact 409, reviewer request, current-head approval, reload and tenant denial passed on disposable services; unqualified merge returned exact 409. After approval, gate remains `unknown` and merge returns 409 because organisation policy and qualification/evidence are missing. |
| P1 / T29v / T02 | OIDC runtime and existing-member connected GUI flow complete; first-user onboarding in progress | Connected save/probe/activate/reload/sign-in/disable passed at runtime commit `08669b4`; see [evidence](t29-identity-active-gui-review.md). New IdP user invitation/onboarding remains in progress. Customer IdP certification remains external; worker-reported PG/race output is not retained. |

T31a before/after launcher probes are `.local/organisation-layout-2026-09-23/protocol-probe.jsonl` and `.local/t31a-acceptance/protocol-probe-after.jsonl`; they exercise classification only. The real container protocol test passed (`.local/t31a-acceptance/container-test.log`). Implementation and exact local evidence: [T31a review](t31a-review-2026-09-23.md). T29s locally completed with focused evidence in [its review](t29s-review-2026-09-23.md). T29t focused keyboard/outside/route/theme checks and full-suite evidence are in [its review](t29t-mobile-navigation-review.md) and `.local/t29t/`. The 14 latest full-suite skips are opt-in scenarios needing disposable integration services, credentials or visual capture; inspect their dispositions before release. No G5 claim follows. Newly found issues belong here and in backlog with reproduction, priority, owner and closure evidence.

## Remaining local work

| Ticket | Current evidence | Next action / state |
| --- | --- | --- |
| T28a | V01–V29/J01–J10/R01–R13 matrix updated. Latest guarded Go/PostgreSQL integration passed; current image still needs rebuild and review. | Run full GUI suite against current GUI revision, finish T29u positive authorized-merge evidence and T29v first-user invitation/onboarding work, and complete exact-current image/source/notice/legal and visual review; external G5 gaps remain. See [reconciliation](t28a-reconciliation-review.md). |
| T28b | 10k repos/10 orgs and 50 browser sessions are locally verified on fixtures. | 100 simultaneously executing jobs still **host blocked**: no writable delegated cgroup v2 on current host; nested Docker cgroup read-only, gVisor failed before guest start. Need isolated delegated Linux runner; broader contention fairness, cancellation/restart and enforced-resource evidence. |
| T28c | Bounded amd64 build/notice check is at `cce6879`; Alpine provenance review is at `af230bd` and compares installed packages in selected local images. Neither proves an image built from current `35b7545` source. | Build/review exact current revision. Historical Alpine inputs and source texts remain incomplete; reproducibility, arm64 and legal disposition remain open. See [image review](t28c-final-image-review.md), [Alpine provenance](t28c-alpine-provenance-review.md), [notice delivery](t28c-notice-delivery-review.md). |
| T27 | Local gVisor development boundary, endpoint, identity, budget and recovery tests pass in bounded cases. | Hosted isolation/resource proof remains blocked on delegated cgroup runner. Do not enable hosted untrusted execution before hostile cross-tenant corpus passes. |
| T31 | Integrated PostgreSQL+gVisor profile lifecycle, profile concurrency, terminal replay, frozen checks and fixture publication gate passed with race detector. | Official runtime/account entitlement, permitted routes, approval interception, protected native publication and hosted isolation remain external. |
| T29 | Full suite at `cce6879` is 154/14. Focused current Policies/Audit/Connections/Organisation checks, picker fixture fix and Overview sync warning are recorded. | Current full suite and integrated human visual review remain open; T29u positive authorized merge remains open; T29v first-user onboarding is in progress. Connected Identity GUI flow passed. |

T26 Compose bootstrap and local container acceptance are verified: guarded role/bootstrap and grants, existing-volume refusal cases, no-runner-dir config, docs route, clean install, migration retry, fixture-off local TLS/OIDC, process restart and encrypted restore. Separate DB process-kill recovery proves workflow/runner lease fencing and reclaim, not Compose restart with active jobs. Customer OIDC/KMS and published-image/runner recovery remain external/deployment-specific. See [T26](t26-compose-review.md), [install](t28a-install-review.md), [active-job review](t28a-active-job-recovery-review.md).

## External certification actions

| Tickets | Input required | Qualification action |
| --- | --- | --- |
| T08–T11, T21–T25 | Dedicated authorised GitHub/GHES and GitLab accounts/instances with claimed tier features; appropriate Gitea instance for wider version claims | Import, bot cooperation, publication/recovery, approvals, queues/trains, strict update, native deployment and campaign scenarios. Existing pinned disposable Gitea proof remains narrowly scoped |
| T12–T15 | BYO model accounts, explicit paid-test budgets and target self-hosted endpoint versions | Real tool loops, interruption, quota, usage and protocol-profile tests; local Ollama does not certify every compatible server |
| T16/T17/T31 | Official runtime/account, permitted entitlement/topology, pinned image and custody/approval evidence | Qualify Codex, Claude Code and `agy` separately, including cancellation, quotas and pre-effect control. Unknown/unsupported routes remain disabled |
| T16/T26/T27 | Customer OIDC/KMS and authorised hosted/private-runner infrastructure | Qualify identity, isolated execution, outbound/private routing, drain/restart and operator recovery. Kubernetes changes only through GitOps |
| T22/T23/T26 | Disposable native delivery environments and GitOps reconciler with health attribution | Preserve provider approvals and certify artifact/environment/health correlation; pipeline completion alone is not health |
| T27/T28b | Disposable Linux runner with delegated writable cgroup v2 and capacity for 100 configured jobs | Run 100 actual concurrent executions, hostile tenant corpus, resource readbacks, fairness, cancellation and restart/recovery; current host cannot provide this boundary |
| T28c/T28/T29 | Current integrated image/build plus product-owner and legal review | Build exact current source; close applicable notice provenance; inspect current visual captures and obtain human baseline approval |
| T28 | All required evidence above; owner release decision | Final support/version matrix and R01–R13/G5 review. Publication/external deployment requires separate authorisation |

## Ticket summary

| Tickets | Current state |
| --- | --- |
| T01–T06 | Locally complete; retained historical evidence |
| T07 | Local runner implemented; hosted isolation qualification open |
| T08–T15 | Local adapters implemented; external/version certification limited as above |
| T16/T17 | Backend-connected implementation present; qualification acceptance incomplete |
| T18–T25 | Locally complete; live/native-provider release evidence incomplete |
| T26 | Local Compose install/role/grant/recovery slice verified; customer/hosted operation remains external |
| T27 | Partial; delegated cgroup hostile/isolation qualification open |
| T28 | Partial; guarded full integration passed; exact-current image, GUI acceptance, open P1 defects and external release qualification remain |
| T29 | T29a–t local slices recorded; T29w Overview warning locally verified; T29u negative-path acceptance complete, positive merge open; T29v connected existing-member Identity GUI flow complete, first-user onboarding in progress; current full suite and human review remain |
| T30 | Docs engine/help locally verified; current image/notice provenance and final release content remain |
| T31 | Integrated PG+gVisor profile lifecycle and T31a locally complete; official runtime and hosted qualification open |

Launch from repository root: copy `deploy/compose/.env.example` to `.env`, set unique URL-safe database role passwords, 32-byte base64 encryption key, HTTPS public origin, OIDC issuer/client secrets, bootstrap token/expiry, and proxy TLS/routes for app plus `/docs/`; then run `docker compose --env-file .env -f deploy/compose/compose.yaml up --build -d`. Open configured origin and `/docs/`. Follow [install guide](../../deploy/docs/content/install.md); do not expose DB or runner ports. Existing local endpoints, when started, are app `http://127.0.0.1:8080` and docs `http://127.0.0.1:8082/docs/`. Use [backlog](backlog.md), [validation](validation.md), and [progress](progress.md) for acceptance, ownership and chronological evidence. The frontend large-chunk build advisory remains a performance follow-up; no measured user-facing failure established.

The corrected support matrix passed the strict MkDocs container build and was verified
served by the existing local docs endpoint. Image `reforge-docs:gui-review`:
`sha256:1d227a08192da49737c62f972b6b0500e706aa4c01b4e993679380ef82fe29e6`.
Build log: `.local/organisation-layout-2026-09-23/docs-build.log`.
