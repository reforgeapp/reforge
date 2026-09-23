# Current work and review status

Updated 2026-09-24. Product updates integrated through `92e1d03`; final default-browser/image refresh running. Earlier full verification remains attributed to `675b0fd`. This index supersedes earlier status snapshots; chronological evidence remains in [progress](progress.md). **G5 has not passed.**

## Current batch

Opus session quota exhausted; reported reset13:10 Sydney. Luna workers finish final browser/demo and positive-merge follow-up; maximum three workers, exclusive ownership. Coordinator reviews every change. Weekly completion check2026-09-23T23:07:41.467Z: **42% remaining**. Below40% starts checkpoint/demo wrap-up; below25% stops jobs. Persistent goal active.

| Work | Status / evidence |
| --- | --- |
| T29v first-user invitation onboarding | Locally complete at `22cea8a`; real signed local IdP, restricted PostgreSQL, normal CSP, new-member callback, replay/revoke, reload, keyboard and390px light/dark passed. API/generated types/admin guide integrated `18c8045`. [GUI review](t29-identity-invitation-gui-review.md), [API review](t29-identity-api-docs-review.md). Customer IdP certification remains open. |
| Audit/Policies review findings | Fixed `8846d9d`;36 focused browser checks and8 theme/viewport probes passed. [Review](t28a-audit-policy-regression-review.md). |
| Shared focus, team cache, pending merge controls | Fixed `e01287c`, `8cb2cac`, `dc9242b`;45/45 focused lifecycle checks passed. Team rename now invalidates both team-cache shapes; pending merge controls retain keyboard focus. [Lifecycle](t28a-lifecycle-regression-review.md), [focus](t29-shared-focus-review.md). |
| Route surfaces |3/3 checks passed;52 route/theme/viewport records plus2 skip-link records, zero axe violations or overflow. `.local/opus-resume/shared/`. |
| Full current browser suite | **193 passed/15 opt-in skipped/0 failed** against clean production build of `675b0fd`, normal CSP.16 light/dark desktop/narrow captures retained; settled policy layout independently reviewed. Browser assets exactly match final container. [Report](t28a-production-browser-review.md). |
| Current control/runner/docs images | Built/verified clean `675b0fd`, linux/amd64: nonroot, strict MkDocs, byte-matched frontend/notices. Original tracked install script passed directly: migrations, rollback/retry, restart, encrypted restore and local OIDC. [Report](t28c-current-images-review.md). |
| Go/security checks | `go test -race ./internal/...` passed; fresh restricted-role PostgreSQL integration passed76.774s. Fresh auth/HTTP race and final invitation HTTP race passed. Logs `.local/opus-resume/integration/`. Initial reused-fixture auth failure retained separately. |

## Outstanding local work and defects

| Priority / ticket | Remaining action |
| --- | --- |
| P2 / T29y,T29z,T28e | Integrated `c4fe426`, `ba5e45b`, `92e1d03`: closed sidebar shadow, unused Unassign reason, five stale opt-in specs. Six backend-connected scenarios passed;12 focused smoke passed/1 skipped. Final full suite/demo refresh pending. [Opt-in review](t28a-optin-browser-review.md). |
| P2 / T29x | Locally fixed `d23be04`/`74957e4`: all team pickers share invalidation; budget units explicit. Regression proved failure before fix and passes afterward;14 focused tests/typecheck/build pass. Final193/15 suite passed. |
| P2 / T28d | Fixed `9ebf273`: TCP readiness excludes temporary initialization server. Original tracked script passed directly from clean final freeze, including install and recovery. |
| P1 / T29u | Complete genuine repair→validation→policy qualification→native approval→authorized protected merge, then browser confirmation. Genuine qwen2.5:7b current-fixture service integration passed81.74s: source-only repair, native current-head approval, protected merge, canonical result recovery, replay and tenant denial; existing caps unchanged. [Pass evidence](t29u-qwen25-review.md). Positive GUI Go repair passed4 turns and published PR1; GUI policy/qualification completed, but persisted changes list lacked new PR after90s. Supported inventory resync follow-up pending; no GUI merge pass. [GUI evidence](t29u-positive-gui-review.md). Service test seeded qualification in-process and does not certify operator/browser journey. Earlier qwen3:4b failure retained in [model review](t29u-current-model-review.md). Paid APIs and synthetic success prohibited. [Reviewer journey](t28a-reviewer-journey-review.md). |
| T28a / T29 | Historical675b0fd default193/15 gate passed. Tracked opt-in corrections integrated and six real backend scenarios passed; original failures retained separately. Final92e1d03 default suite running. Remaining: positive connected merge and external/human acceptance. |
| T28c / T30 | Final current image/install reviewed. Control/runner28 exact APK signatures/checksums,21 exact-commit APKBUILD declarations, and159/159 declared source payload checksums verified (independent root recheck). Docs-image23/27 historical binary archives recovered and all27 source inputs mapped;4 archives, delivered notice mapping, reproducibility, arm64 and legal disposition remain open. [Inventory](t28c-inventory.md), [provenance](t28c-alpine-provenance-review.md), [exact inputs](t28c-current-alpine-inputs-review.md), [source payloads](t28c-source-payloads-review.md), [docs inputs](t28c-docs-inputs-review.md). |
| T27 / T28b | Prior blanket host blocker superseded: disposable delegated cgroup subtree enabled one production-config/rootless guest with per-run readbacks and cancellation. Fresh-helper memory/CPU/PID pressure, timeout/caller cancellation and SIGKILL orphan recovery passed; measured memory peak12KiB over nominal limit retained. [Enforcement](t27-cgroup-enforcement-review.md).100 concurrent executions, hostile tenant/resource corpus and deployment topology remain unverified; repository/browser fixture scale does not substitute. [Probe](t27-delegation-probe-review.md). |
| T31 | Local profile/protocol slices pass; official runtime entitlements, topology, approval interception and hosted qualification remain external. Unsupported/unverified routes stay disabled. |

Resolved defects retained in linked reports: T29q refresh/cache lifecycle; T29r form alignment; T29s truthful repository-scope state; T29t mobile focus/dismissal; T29w actionable Overview sync warning; T31a strict terminal-result parsing. Final full-browser suite covers repaired regressions; opt-in and certification limits remain explicit.

T26 earlier Compose bootstrap/install/migration/restart/encrypted-restore evidence remains valid for its recorded source. Separate process-kill tests cover active workflow/runner lease recovery; they do not establish active-job recovery across a current Compose/runner restart. Current-image clean install/recovery passed; customer OIDC/KMS and published deployment recovery remain external.

## External certification actions

| Tickets | Input required | Qualification action |
| --- | --- | --- |
| T08–T11, T21–T25 | Dedicated authorised GitHub/GHES and GitLab accounts/instances with claimed tier features; appropriate Gitea instance for wider version claims | Import, bot cooperation, publication/recovery, approvals, queues/trains, strict update, native deployment and campaign scenarios. Existing pinned disposable Gitea proof remains narrowly scoped |
| T12–T15 | BYO model accounts, explicit paid-test budgets and target self-hosted endpoint versions | Real tool loops, interruption, quota, usage and protocol-profile tests; local Ollama does not certify every compatible server |
| T16/T17/T31 | Official runtime/account, permitted entitlement/topology, pinned image and custody/approval evidence | Qualify Codex, Claude Code and `agy` separately, including cancellation, quotas and pre-effect control. Unknown/unsupported routes remain disabled |
| T16/T26/T27 | Customer OIDC/KMS and authorised hosted/private-runner infrastructure | Qualify identity, isolated execution, outbound/private routing, drain/restart and operator recovery. Kubernetes changes only through GitOps |
| T22/T23/T26 | Disposable native delivery environments and GitOps reconciler with health attribution | Preserve provider approvals and certify artifact/environment/health correlation; pipeline completion alone is not health |
| T27/T28b | Representative supported Linux runner topology and capacity for deployment-grade execution certification | Run 100 actual concurrent executions, hostile tenant corpus, resource readbacks, fairness, cancellation and restart/recovery. Current disposable local delegation proves bounded single-guest enforcement/recovery only; broader topology claims require separate evidence |
| T28c/T28/T29 | Current integrated image/build plus product-owner and legal review | Current source build verified; close remaining notice provenance, inspect captures and obtain human baseline approval |
| T28 | All required evidence above; owner release decision | Final support/version matrix and R01–R13/G5 review. Publication/external deployment requires separate authorisation |

## Review artifacts and launch

Current batch artifacts: `.local/opus-resume/`; source and evidence remain local. Demo: `http://127.0.0.1:8084`; docs: `http://127.0.0.1:8082/docs/`. Explicit fixture authentication, real local PostgreSQL, production web bundle. Final freeze serving in Docker; restart with `sh .local/opus-resume/demo/start.sh`, stop app with `sh .local/opus-resume/demo/stop.sh` (Docker required, DB/docs retained); [browser review](t28a-production-browser-review.md) records source, start/stop commands and screenshot provenance. Superseded Vite5173/5174 and backend8080 stopped.

Self-hosted launch: follow [Install](../../deploy/docs/content/install.md), configure `deploy/compose/.env.example`, then `docker compose --env-file .env -f deploy/compose/compose.yaml up --build -d`. Use configured HTTPS origin and `/docs/`; keep database and runner ports private. No external deployment/publication authorized.
