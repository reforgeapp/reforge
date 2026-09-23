# Solo resume handoff

## Current GUI handoff — 2026-09-23

Latest follow-up: T29p shared surfaces/search locally complete. See [shared-controls review](shared-controls-review-2026-09-23.md) for current assets, container and verification. Prior switcher evidence below is historical; its behavior remains covered.

Previous follow-up: T29j organisation switcher reworked after owner screenshots; 17 shell/theme checks passed at that checkpoint. See [switcher review](organisation-switcher-review-2026-09-23.md) for menu-specific evidence.

Owner resumed GUI work; pause below is historical. T29g–o locally complete after Astra
review and Luna repairs. Full browser suite: 133 passed, 0 failed, 9 opt-in skipped;
real policy/budget/audit controls also passed. Deepseek's Campaign/GitOps work retained
with compact filter correction; Findings close/focus, organisation menu, dark palette and
Policies workspace repaired. Impact preview uses stored evidence and real simulation APIs.

Start from [current progress](progress.md) and [GUI feedback review](gui-feedback-review-2026-09-23.md)
for exact artifacts, image identities, live demo and remaining work. Preserve owner edits
to `copilot-handoff-2026-09-22.md`; its old ticket states are superseded by this checkpoint.
T16/T26 certification, T27 hosted isolation, T28 actual executing-run/browser load and
release/dependency review remain open. No G5, human approval or external certification claimed.

## Historical usage pause — 2026-09-22 (superseded)

Do not continue automatically. Owner requested a usage pause after closing the current workers. Resume from [Copilot/Luna tickets T29g–n](copilot-handoff-2026-09-22.md), which supersede earlier stopping instructions and identify pending work, file ownership and actual checks. Keep the current local demo available. Do not integrate the parked policy impact experiment as completed functionality.

## Authoritative handoff — 2026-09-22

The prior solo-complete narrative is superseded by the agent-reviewed T29 GUI review. Continue from current source and evidence. GUI rebuild is locally verified; T29 acceptance remains open for documented policy workflow gaps: automatic stored-evidence/portfolio simulation. See [GUI policy requirements](../design/gui.md). T30/T31 and release qualification remain evidence-bound. Do not claim human sign-off, G5 completion, external provider certification or external OIDC certification.

Verified current evidence: `go test ./...` and `make check` pass; browser final `.local/rebuild-browser-final.json` records 108 passed, 0 failed, 9 opt-in skipped; policy/insights `.local/policy-final.json`; persisted policy activation, zero budget and audit export `.local/live-controls.json`; fresh 26-route and eight-detail captures pass in `.local/rebuild-final/`; full authority and launch details are in [the GUI rebuild review](gui-rebuild-review-2026-09-22.md). The historical `83/24/9` full-suite result is retained for comparison only.

## Historical handoff note — superseded 2026-09-22

This historical note records an earlier claim. Current T29 status and evidence are defined by the authoritative section above and current review document; do not use the old HEAD or completion claim. All content under this note is historical evidence.

Review covered the three existing artifacts `.local/visual/runners-1440.png`, `connections-1440.png` and `runs-1440.png`, not a fresh live-browser review of every route. Findings: shell organization remains materially unchanged; runner view lacks searchable/filterable inventory and operational heartbeat/capacity/trust summary, while current pool pagination is cursor-based Load more; it uses raw repository IDs. Connections has a large revoked table before profile workflows; Runs is empty despite populated-state claims. Existing selector/heading counts, axe output and 700ms screenshot delay are necessary checks only, not evidence of coherent design or populated data.

Resume T29 through children T29a–f in backlog: IA/wireframes; shared resource layouts; Runners/Connections pilot; remaining routes/auth/help; workflow/accessibility/responsive states; comparative visual review. Use same route/data/state before/after at 1440x900 and 390x844, with built/served asset identity and cache reset. Do not wait for human sign-off or stop after the pilot; propagate through all routes. Keep APIs/security and backend gains. Raw JSON is advanced policy export only; named controls are primary.

| Ticket | Current status | Evidence boundary / next action |
| --- | --- | --- |
| T29 | local complete (agent-reviewed) | T29a–f implemented: IA map, shared toolbar/split layouts with URL detail, Runners/Connections slices, all inventory/workspace routes and auth/help, state/accessibility/responsive coverage, comparative review and a labelled demo seed. |
| T30 | reported local complete | Preserve reported docs build evidence; recheck links after T29 route changes. |
| T31 | implemented, reported extraction/validation; focused regression pending | Patch is reported integrated; inspect evidence and run focused regression checks. |
| T26 | partial | Local OIDC/TLS evidence reported passed; hosted/customer OIDC remains separate external certification. |
| T28 | partial | Load notes do not prove 100 executing runs or 50 real browser sessions; release qualification remains open. |

All rows and claims below are historical. Reconcile them against current code and evidence before marking any ticket complete.

Updated: 2026-09-21 historical checkpoint. Single implementing agent, no workers, no subagents.

## Checkpoint

- Starting HEAD: `2a40587 docs(handoff): record T29 work-surface batch` (working tree
  clean; no uncommitted paths at start).
- Earlier handoff revisions recorded `6ef9025` as the start; `2a40587` is the latest
  checkpoint and its ticket table/commit list are the reconciled baseline. No reset or
  checkout of `6ef9025` was performed.
- The `go build` / `BotRevalidation.task_id` generation failure described in the original
  checkpoint was already fixed and committed before `2a40587`; re-verified by a clean
  `make build`.
- Server running from `./bin/reforge` on `127.0.0.1:8080`; PostgreSQL 18 on
  `127.0.0.1:55432`. See "Running the local stack" for start/stop.

## Final commits

| Commit | Content |
| --- | --- |
| `7815b0c` | gofmt import order in `internal/httpapi/discovery.go` |
| `9215b9f` | T25 bounded portfolio campaigns (inherited work, reviewed and completed) |
| `32c365b` | T29 increment: route header gate, Overview aggregate/route, Organisation route, help drawer |
| `3306efd` | T30 versioned MkDocs site, docs container, `REFORGE_DOCS_URL` help links |
| `e0713ee` | T26 partial: control/runner/docs images, Compose stack, `.dockerignore` |
| `f02e762` | T31 approved custom command profiles, executor and container test |
| `0a8d040` | T16/T17 agent qualification records, capability endpoint and GUI panel |
| `310d550` | T26 fixes: PostgreSQL 18 volume path, private artifact directory; container verification |
| `24dde46` | T29 overview portfolio/capacity + visual capture harness |
| `c7a440a` | T29 runs work surface + server-side state filter |
| `21b7f31` | T29 changes work surface |
| `a24a8ae` | T29 deployments timeline |
| `5dd6b5b` | T16 `agy` distinct official runtime provider |
| `d6b0117` | T29 drop repeated card headings on Usage/Policies/Runners |
| `fb9f081` | T29 narrow shell overflow, scrollable table labels, route-surface gate |
| `9e52ea2` | T29 drop campaign and custom-profile card headings |
| `6643005` | T29 per-route on-demand help-link test |
| `f2e713f` | T31 controller-to-runner custom profile dispatch |
| `ba5efb5` | T16 runtime factory and managed login/logout plumbing |
| `79834b6` | T26 encrypted restore drill |
| `1716f93` | T26 hosted GitOps reference manifests |
| `350f244` | T29 per-route error-state visual captures |
| `a68e1f2` | T28 support matrix: custom profile limits and verification identity |
| `e28eea3` | T27/T29 handoff evidence |
| `5ebc8e5` | T28 dependency and notice inventory refresh |
| `442b4ee` | T31 custom profile output advances to validated repair |
| `fb6d67a` | T29 visual-review fixes (duplicate titles, budget group) |
| `70208d3` | T26 non-development install with local OIDC/TLS |
| `300391f` | T28 control-plane scale harness |
| `9486b9d` | T27 reproducible sandbox hostile corpus target |
| `e385a49` | T16 digest-pinned runtime delivery |
| `480cf9f` | handoff ticket-status consolidation |
| `7eaa3d7` | T29 blocked/stale visual captures |
| `107b335` | T28 licence review outcome |
| `dee4965` | T17 self-hosted bootstrap GUI |
| `6973e24` | docs container root redirect / port preservation |
| `c8f9a61` | T29 ground-up design system and shell rebuild |
| `c7b109d` | T29 toolbar/list/active-control refinement + all-route axe |
| `d7f66ef` | T29 sign-in and detail captures |
| `c8f9a61` | T29 design system and shell rebuild |
| `c7b109d` | T29 toolbar/list/active-control refinement |
| `d7f66ef` | T29 sign-in and detail captures |
| `b00e929` | stop caching the SPA entry (review baseline) |
| `bcc479b` | T29c runners pool inventory and persistent detail |
| `c9e9a47` | T29c tabbed connections inventory and profile detail |
| `29c009e` | T29d findings split detail |
| `2f4d54c` | T29d runs split detail |
| `cafa597` | T29d changes split detail |
| `47ab11e` | T29d deployments split detail |
| `b877821` | T29d audit split detail |
| `2460285` | T29d repositories split detail |
| `f3ccc98` | T29d structured policy simulation controls |
| `1991b2a` | T29d campaigns split detail |
| `e97a811` | T29 demo seed |
| `fb4f560` | T29f comparative review |
| final HEAD | `fb4f560` (see `git log`) |

Inherited uncommitted paths preserved and committed in `9215b9f`/`7815b0c`:
`api/openapi.yaml`, `cmd/server/main.go`, `internal/deployment/{cancel,operations,service}.go`,
`internal/gitops/{authority,publish,service}.go`, `internal/httpapi/generated/models.go`,
`internal/httpapi/server_test.go`, `internal/maintenance/recipes/presets.go`,
`internal/maintenance/repair/service.go`, `internal/store/dbgen/models.go`,
`internal/workflow/service.go`, `web/src/api/schema.ts`, `web/src/app/{SectionPage,UsagePage}.tsx`,
`web/src/main.tsx`, `web/src/styles/app.css`, plus untracked campaign files and migration 029.

## Ticket status

| Ticket | Status | Notes |
| --- | --- | --- |
| T16 | partial (externally gated) | Agent qualification, distinct `agy` provider, runtime launcher factory, digest-pinned host/container runtime delivery, and managed official login/logout behind owner/admin authority, current qualification, a durable audit claim and fail-closed disabled states. Live account/entitlement and hosted isolation evidence remain external. |
| T17 | partial | Connection/agent qualification GUI is backend-connected and shows disabled reasons. Forge/model/runner qualification flows pre-existed; live SaaS/OSS probe evidence remains external. |
| T25 | local complete | Campaign controller/authority/execution reviewed; PG race and browser suites pass. External native-provider campaign acceptance not run. |
| T26 | partial (externally gated) | Control/migrator/runner/docs images, Compose install, hosted GitOps reference (`deploy/gitops`), encrypted restore drill, and a non-development install check with local OIDC/TLS (fixture auth disabled, browser login verified). Hosted cluster/customer OIDC certification remains external. |
| T27 | partial | Integrated race suite over isolation/concurrency/recovery passes; reproducible gVisor hostile corpus (traversal, symlink, corrupt fetch, cancellation) passes locally; 10,000-repo import, 50-session and 100-claim load harness recorded. Hosted cgroup-enforced isolation and live-provider certification remain external. |
| T28 | partial | Support matrix, verification identity, dependency/notice inventory, restore/install/GitOps limits and the scale harness are recorded. Final release sign-off, licence review of remaining "review required" transitive dependencies and hosted certification remain. |
| T29 | local complete (agent-reviewed) | Ground-up design system, shell and structural list/detail rebuild across every route; agent-reviewed captures; human aesthetic feedback is optional and not a gate. |
| T30 | local complete | MkDocs `--strict` build passes; docs container verified serving current/archived versions and search; every route links contextual help. |
| T31 | local complete | Versioned admin-approved profiles, protocol executor, real container test, controller-to-runner dispatch with durable budget reservation and revocation fencing, and end-to-end advancement: a `completed_unverified` profile run has its changed source extracted and passed through the frozen baseline/candidate/target checks, then staged and published by the existing processor. Turn events are bounded by `max_turns`; exit 0 alone never publishes. |

## Architecture decisions

- `BotRevalidation.task_id` uses `x-go-type: string` to keep the generated contract
  dependency-free and consistent with the rest of the UUID surface.
- Overview counts are computed server-side in `internal/insights/overview.go` under the
  live actor scope, rather than assembled from paginated client lists.
- Help links read `REFORGE_DOCS_URL` from `/api/v1/meta`; the docs container serves the
  current site at `/docs/` and archived copies at `/docs/<version>/`.
- Container images are pinned multi-stage builds; the runner image has no Docker socket
  and no baked credentials.

## Checks actually run

- `make check` — pass (gofmt, `go vet ./...`, frontend production build, generation drift).
- `go test -race -count=1 -timeout 25m ./test/integration/...` — final pass `82.591s`
  against disposable PostgreSQL 18.6 (migration applied first).
- `go test -count=1 ./...` — pass.
- Campaign integration subset `Campaign|DeploymentContinue` — pass `10.403s`.
- Browser suite `npx playwright test --grep-invert live` against `127.0.0.1:8080` —
  `90 passed, 1 skipped`. New specs: `web/tests/overview.spec.ts`,
  `web/tests/organisation.spec.ts`, `web/tests/custom-profiles.spec.ts`,
  `web/tests/agent-qualification.spec.ts`.
- Custom command container test: `REFORGE_TEST_DOCKER=1 go test -race -run TestRealContainerProfileProtocol ./internal/customcmd/`
  — 6 subtests pass (input/output, malformed, nonzero exit, timeout, cancellation, secret
  isolation).
- `mkdocs build --strict` (material 9.7.0, local deps in `/tmp/mkdocs-deps`) — pass,
  `search/search_index.json` generated.
- Docker (daemon available this session): docs/control/runner images build. Docs container
  served `/docs/`, `/docs/agents/`, `/docs/0.1.0/agents/`, `/docs/versions.json` and
  `search_index.json` (200). Control container ran as uid 10001, no
  `/var/run/docker.sock`, no mounts, `/readyz` 200, SPA and meta served after migrating a
  clean database.
- Live endpoint probes: overview counts; custom-profile create/list/approve/revoke; agent
  qualification PUT returned feature capabilities.

Solo resume session (`2026-09-21`, commits `5dd6b5b`…`fb9f081`):

- `go test ./internal/connections/ ./internal/agent/` — pass after adding `agy`.
- `web` production build (`npm run build`, tsc + vite) — pass.
- `npx playwright test visual.spec.ts` with `REFORGE_VISUAL=1` against
  `127.0.0.1:8080` — pass; captures in `.local/visual/`, `metadata.json` records one h1
  and primary-toolbar count per route at 1440x900 and 390x844.
- New `web/tests/route-surface.spec.ts` — pass; enforces exactly one `h1`, at most one
  `.repository-toolbar`, 390px no horizontal overflow for all 13 routes, and axe clean on
  findings/policies/usage/audit/runners. Caught and fixed a real 14px topbar overflow and
  an unlabelled scrollable table region.
- Full browser suite `npx playwright test --grep-invert live` against `127.0.0.1:8080` —
  final `93 passed, 2 skipped`.
- Targeted regression subset (`insights`, `repositories`, `organisation`, `connections`)
  — 15 passed after the card-heading removal.
- `go test -count=1 ./...` with the disposable test database — exit 0 (no failures).
- `make check` (gofmt, `go vet ./...`, frontend build, generation drift) — exit 0.
- T31 real container protocol:
  `REFORGE_TEST_DOCKER=1 go test -race -count=1 -run TestRealContainerProfileProtocol ./internal/customcmd/`
  — pass `3.882s`.
- T16 agent qualification integration
  `go test -count=1 -run TestAgentQualificationGatesCapabilities ./test/integration/` —
  pass `0.599s`.
- T31 dispatch integration
  `go test -count=1 -run TestCustomProfileDispatchAuthorizesExecutesAndFencesRevocation ./test/integration/`
  — pass `0.862s`; exercises preview binding, durable budget reservation, authorize, report
  settlement and revoked-profile fencing against disposable PostgreSQL.
- T31 real container protocol re-run (see above) — pass.
- T16 auth gating
  `go test -count=1 -run TestAgentAuthStaysDisabledUntilRuntimeAndQualificationExist ./test/integration/`
  — pass.
- T26 restore drill `scripts/restore-drill.sh` against disposable PostgreSQL — pass:
  `tables=73 migrations=33` and `TestRestoreDrillKeyRecovery` pass.
- `kubectl kustomize deploy/gitops/overlays/example` — renders 6 resources.
- `mkdocs build --strict` after the GitOps/install doc update — pass.
- Full Go suite `go test -count=1 ./...` — exit 0; `make check` — exit 0.
- Full browser suite `npx playwright test --grep-invert live` — `94 passed, 2 skipped`.
  New `repair-preview.spec.ts` custom-profile selection test included. The
  `connections.spec.ts` pool test is now pagination-tolerant after a repeated-run flake.
- T27 integrated qualification run:
  `go test -race -count=1 -timeout 25m ./test/integration/...` — pass `116.359s` against
  disposable PostgreSQL 18.6, covering cross-tenant/team isolation, RLS under pooling,
  lease/queue/outbox recovery, budget contention, webhook replay/rotation, revoked
  identities, repair/publication reconciliation, merge recovery and custom dispatch.
- T29 visual harness re-run with `REFORGE_VISUAL=1` — pass `43.8s`: 26 populated captures
  (13 routes × 1440/390) plus 13 actionable error-state captures; zero one-title/one-toolbar
  gate violations.
- `mkdocs build --strict` after the support-matrix update — pass.

Acceptance continuation (`2026-09-22`, commits `442b4ee`…`e385a49`):

- T31 end-to-end: `TestValidateCustomRequiresBaselineAndTarget`, `TestCustomProfilePatchExtraction`
  and `TestTurnBudgetIsEnforced` pass; the custom path now extracts changed source, runs the
  frozen checks and continues to stage/publish.
- T26 install: `scripts/install-check.sh` (`make install-check`) passes end-to-end against
  disposable PostgreSQL with a local OIDC issuer and TLS proxy, fixture auth disabled:
  production `/api/v1/meta`, OIDC redirect, authorization-code login, session and browser
  login all verified.
- T27 isolation: `make sandbox-test` passes the real gVisor hostile corpus (traversal,
  symlink escape, truncated/corrupt fetch, cancellation, development boundary) with the
  pinned `runsc`.
- T28 scale: `REFORGE_LOAD=1 go test -run TestControlPlaneLoadTargets ./test/integration/`
  passes; measured 10,000-repo scan `2.6s`, import `9.8s`, first page/search `5ms`,
  50-session p50/p95/max `16/19/20ms`, 100 claims `2.0s`.
- T29 ground-up redesign (owner feedback: the previous consistency pass was not a
  redesign): replaced the token palette, added an inline icon set, redesigned the
  sidebar/topbar, and restyled cards, toolbars, tables, forms, status badges, dialogs,
  sign-in and bootstrap. Screenshots opened and inspected for every route at desktop and
  390px; fixed contrast (axe) and narrow-width overflow the new layout exposed; extended
  the route gate to run axe across all 13 routes. `web/tests/visual-states.spec.ts`
  captures sign-in, a finding detail, a blocked run and a stale repository.
- T29 early pass (retained): fixed duplicate Overview/Findings/Campaigns titles, labelled
  the budget group, removed repeated card headings and generic intros.
- T16 runtime delivery: digest-pinned host launcher and digest-pinned container launcher
  added with unit tests rejecting mutable tags, relative executables and credential env.
- T17 bootstrap GUI: `web/tests/bootstrap.spec.ts` passes; the self-hosted no-organisation
  dead end is now the one-time bootstrap form.
- T28 licence review: no GPL/AGPL runtime dependency; MPL-2.0 build/test dependencies are
  compatible with notice retention; `review required` transitive modules recorded.
- `make check`, `go test ./...` (exit 0), full browser suite `--grep-invert live`
  (`95 passed, 4 skipped`), `make sandbox-test`, `make install-check`, the load harness
  and `go test -race ./test/integration/...` (`117.416s`) all pass after these changes.
- Container re-verification after the changes: `docker build` succeeds for the control,
  runner and docs images; the control container runs as uid 10001 with no
  `/var/run/docker.sock`, serves `/readyz` 200 and `/api/v1/meta`, then is removed.

T29 structural rebuild (`bcc479b`…`fb4f560`):

- T29a IA/wireframes (`docs/implementation/t29a-ia.md`); T29b shared `Toolbar`/`SplitView`
  primitives; T29c Runners (API-backed pool counts, runner heartbeat/busy/route counts,
  pool search/filter, get-pool endpoint, named repository picker) and Connections (Forges /
  Models & agents / Delivery tabs, profile detail).
- T29d converted Findings, Runs, Changes, Deployments, Repositories, Campaigns, Audit to
  URL-addressable split detail and replaced policy rollout/binding JSON with named controls.
- T29e: full browser suite `--grep-invert live` `95 passed, 5 skipped`; route gate axe clean
  on every route at 390px; error captures per route plus blocked/stale captures.
- T29f comparative review with a labelled demo organisation (`make demo-seed`) and
  before/after captures at 1440x900 and 390x844 (`t29f-review-2026-09-22.md`).
- `go test ./...` and `make check` re-run green after the runner/pool API and OpenAPI changes.

Remaining-ticket focused re-runs (2026-09-22, after the T29 rebuild):

- T31: `TestCustomProfileDispatchAuthorizesExecutesAndFencesRevocation`,
  `TestValidateCustomRequiresBaselineAndTarget`, `TestTurnBudgetIsEnforced`,
  `TestCustomProfilePatchExtraction` pass; real container protocol
  (`REFORGE_TEST_DOCKER=1`) pass `4.315s`.
- T26: `scripts/install-check.sh` pass (production mode, OIDC login, browser login).
- T27: `make sandbox-test` (real gVisor hostile corpus) pass `7.169s`;
  `go test -race ./test/integration/...` pass `139.769s` including the new runner pool API.
- T28: load harness pass (10,000-repo import `2.9s/10.5s`, 50 sessions p50/p95/max
  `19/21/22ms`, 100 claims `2.2s`). The 100-executing-run and 50-real-browser-session
  targets remain unproven and are recorded as open, not passed.
- `make check` 0 and `go test ./...` 0 after the runner/pool API, OpenAPI and demo-seed changes.

Not run: hosted cgroup-enforced isolation, live provider/agent-account certification,
customer OIDC/hosted-cluster install, and the 50-real-browser-session variant of the load
harness (the harness uses 50 concurrent authenticated HTTP sessions).

## Running the local stack

Historical stack checkpoint: development server pid `2843697` on `127.0.0.1:8080`,
log `.local/server-solo.log`; PostgreSQL pid `2312959` on `127.0.0.1:55432`. If that
historical stack is still present, stop those exact PIDs with `kill 2843697` and
`/tmp/reforge-postgres/bin/pg_ctl -D .local/postgres stop -m fast`. Start it again with:

```sh
set -a; . .local/development.env; set +a
export REFORGE_MODE=development REFORGE_FIXTURE_AUTH=true REFORGE_EDITION=self-hosted \
       REFORGE_ADDRESS=127.0.0.1:8080 REFORGE_PUBLIC_URL=http://127.0.0.1:8080
setsid --fork ./bin/reforge >.local/server-solo.log 2>&1 </dev/null
```

Current verified services: PostgreSQL PID `2952266` on `127.0.0.1:55432`, application PID
`2976298` on `127.0.0.1:8080`, and docs container `reforge-review-docs` on
`127.0.0.1:8082`. Stop only exact services with `kill 2976298` and
`/tmp/reforge-postgres/bin/pg_ctl -D .local/postgres stop -m fast`; stop docs with
`docker stop reforge-review-docs` when present. Recheck each PID/command before stopping.

Browser tests:

```sh
REFORGE_BASE_URL=http://127.0.0.1:8080 \
PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome \
npx playwright test --grep-invert live
```

Docs build:

```sh
cd deploy/docs && PYTHONPATH=/tmp/mkdocs-deps python3 -m mkdocs build --strict -d /tmp/reforge-docs-site
```

## Known defects and limits

- T29 is incomplete: a route-surface regression gate, per-route help-link test and visual
  captures with route/runtime metadata now exist, and the repeated card headings were
  removed from Usage, Policies, Runners, Campaigns and the custom-profile panel. Still
  outstanding: reviewed baselines for populated/empty/error/blocked/stale captures per
  route family, and a pass over the remaining explanatory copy in Connections.
- T31 dispatch is wired and locally verified. The v1 profile protocol is a single bounded
  invocation, so `max_turns` is declared and validated but not looped, and a profile run
  records `handoff` rather than a validated repair.
- T16 runtime factory and managed login/logout are wired but stay disabled until an
  operator supplies an isolated runtime path; no live account evidence exists.
- Agent qualification records the seven checks but does not itself certify the runtime;
  the runtime entry intentionally stays `unsupported`.
- `web/tests/merge-settings.spec.ts` and `web/tests/deployments.spec.ts` had brittle
  locators from earlier route additions; locators were tightened. `merge-settings`
  non-Gitea now waits for the primary connection provider before saving.

## External actions required

- OIDC issuer/client and an HTTPS public origin to verify a non-development Compose
  install (development mode requires host loopback and cannot run behind port mapping).
- Sandbox host with working cgroup delegation for hosted untrusted execution.
- Live GitHub/GitLab test organisations and credentials for provider certification.
- Official agent accounts only where terms and topology permit; Codex/Claude/`agy`
  qualification evidence.
- Paid model test budgets if direct-API certification is required.

## Review boundary

Review `7815b0c`, `9215b9f`, `32c365b`, `3306efd`, `e0713ee`, `f02e762`, `0a8d040`,
`310d550`, `5dd6b5b`, `d6b0117`, `fb9f081`, `9e52ea2`, `6643005`, `f2e713f`,
`ba5efb5`, `79834b6`, `1716f93`, `350f244`, `a68e1f2`. The T25 commit is large and includes inherited
uncommitted work; review the campaign authority callbacks in
`internal/campaign/authority.go` and `execution.go` first.
For T31 review `internal/customcmd/{executor,rules,service}.go` and the container test;
confirm the sandbox stdin addition in `internal/sandbox/runtime_linux.go` is bounded. For
T16 review `internal/agent/qualification.go` and `internal/httpapi/agentqualification.go`,
especially the binding derived from the connection rather than the request body, plus the
new `agy` provider validation in `internal/connections/service.go`.
T27/T28 qualification evidence is consolidated in
`docs/implementation/t27-qualification.md`; the scale numbers are in
`deploy/docs/content/support-matrix.md`.
For T29 review `web/tests/route-surface.spec.ts`, `web/tests/visual.spec.ts`,
`web/src/components/DataTable.tsx`, the topbar rules in `web/src/styles/app.css`, and the
removed headings in `web/src/app/{UsagePage,PoliciesPage,RunnersPage,CampaignsPage,CustomProfilesPanel}.tsx`.
For T31 review `internal/customcmd/dispatch.go`, `internal/runnerclient/processor.go`
(`runCustomProfile`), `internal/maintenance/repair/{service,types}.go` and migrations
032/033; confirm the budget reservation is committed before the profile starts and that
`Report` cannot settle a different attempt.
For T16 review `internal/agent/{factory,auth}.go`, `internal/agent/process_linux.go`
(`NewCommandLauncher`) and `internal/httpapi/agentauth.go`; confirm the launcher is only
reachable through an operator-supplied absolute path and that login/logout fail closed
without qualification.

## Completion ledger — solo resume 2026-09-21

Criterion-by-criterion status. "Inherited" means evidence existed before this resume and
was not re-run here; "re-run" means a check executed in this session. External
certification is always separate from local completion.

| Ticket | Status | Acceptance check | Remaining / prerequisite |
| --- | --- | --- | --- |
| T01 | local complete (inherited) | build, migration, generation, unit/integration | External: none |
| T02 | local complete (inherited) | OIDC/scopes/bootstrap, PG race | Live OIDC provider |
| T03 | local complete (inherited) | envelope/KMS, write-only secrets, routes | Live AWS KMS |
| T04 | local complete (inherited) | shell, keyboard, deep links | — |
| T05 | local complete (inherited) | leases/fences, budgets, SSE replay | — |
| T06 | local complete (inherited) | policy corpus, immutable versions | — |
| T07 | local complete (inherited) | sandbox isolation, private transport | Hosted cgroup host |
| T08 | local complete (inherited) | GitHub adapter, protection | Live GitHub test org |
| T09 | local complete (inherited) | GitLab adapter, trains | Live GitLab group/instance |
| T10 | local complete (inherited) | Gitea adapter, real local server | — |
| T11 | local complete (inherited) | inventory, webhooks, 1000-repo import | Live forges |
| T12 | local complete (inherited) | OpenAI streaming/tools | Paid API budget |
| T13 | local complete (inherited) | Anthropic messages/tools | Paid API budget |
| T14 | local complete (inherited) | Google continuation | Paid API budget |
| T15 | local complete (inherited) | compatible protocol, real Ollama | vLLM host |
| T16 | partial | `agy` provider, runtime factory and managed login/logout plumbing (re-run integration pass) | Operator-supplied isolated runtime and live account/entitlement evidence |
| T17 | partial | connection/agent qualification GUI | SaaS/OSS qualification matrix; live probes |
| T18 | local complete (inherited) | bot ownership/dedup | — |
| T19 | local complete (inherited) | repair loop, trusted validation | Larger model for upgrade fixture |
| T20 | local complete (inherited) | findings/runs/changes GUI | — |
| T21 | local complete (inherited) | merge controller, native proof | Live provider certification |
| T22 | local complete (inherited) | CI/CD orchestration | Live native approvals |
| T23 | local complete (inherited) | GitOps promotion | Hosted GitOps reference (T26) |
| T24 | local complete (inherited) | policy/deployment/usage/audit GUI | — |
| T25 | local complete (inherited) | campaigns, fairness | External native-provider campaign run |
| T26 | partial | control/migrator/runner/docs images, Compose install, GitOps reference, restore drill and local OIDC/TLS evidence reported pass | Hosted/customer OIDC, hosted isolation and operator agent runtime image |
| T27 | partial | integrated race suite over isolation/concurrency/recovery (re-run pass) | Hostile-repository corpus on a cgroup-capable host, load gate, clean-install/upgrade qualification |
| T28 | partial | support matrix, verification identity and restore/GitOps limits documented | 10,000-repo/100-run load gate, licence/dependency inventory rerun, non-development install, operator handoff sign-off |
| T29 | local complete (agent-reviewed) | shared list/detail layouts, Runners/Connections slices, all routes converted, comparative captures (`t29f-review-2026-09-22.md`) | Human aesthetic feedback only (not a gate) |
| T30 | local complete (inherited) | MkDocs `--strict`, docs container, help links (re-run route help-link test) | — |
| T31 | implemented, reported; focused regression pending | profiles/executor/container plus dispatch and extraction/validation integration reported | inspect current evidence; do not infer failure from stale handoff wording |

Validation matrix: V01–V05, V07–V18, V20–V22 inherited local evidence; V06 (official
agent runtime) and V19 (clean install/restore) partial; V23/V24/V26 partially re-run here;
V25/V27 not satisfied. G5 remains open.

### Stopping condition

**Local work exhausted for the authorised acceptance scope; external acceptance blocked.**
T29a–f are implemented and agent-reviewed; T31 focused regression, T26 install-check and
T27 sandbox/race checks pass. The remaining unsatisfied criteria need inputs or resources
not available here:

1. T28: 100 fully executing runs and 50 real browser sessions. The harness measures 100
   claim lifecycles and 50 authenticated HTTP sessions; proving 100 executing runs needs a
   runner fleet or a job-execution simulator, and 50 browser sessions needs browser
   resources. Recorded as open, not passed.
2. T16/T26: hosted cgroup-enforced isolation host, a live official agent account/runtime
   and entitlement evidence, customer OIDC issuer and a hosted cluster.
3. T08/T09/T12–T15/T22/T23: live forge, paid model and native-approval certification.
4. T28: human release sign-off and licence confirmation of `review required` transitive
   dependencies.

`~/repos/.claude` and all non-empty directories are preserved.

No `.cc-writes`/`.claude` cleanup was needed at this checkpoint; `/home/mnorris/repos/.claude`
and all non-empty directories were preserved.
