# Solo resume handoff

Updated: 2026-09-21. Single implementing agent, no workers, no subagents.

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
| final HEAD | `6643005` (see `git log`) |

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
| T25 | local complete | Committed. Campaign controller/authority/execution reviewed; PG race and browser suites pass. External native-provider campaign acceptance not run. |
| T29 | partial | Header gate, Overview (portfolio + capacity), Runs (server-side state filter), Changes and Deployments work surfaces rebuilt; Organisation connected; visual capture harness stores route/runtime metadata. Findings/Policies/Usage/Audit/Runners/Repositories consistency pass and approved baselines remain. |
| T30 | local complete | MkDocs `--strict` build passes; docs container verified serving current/archived versions and search. |
| T26 | partial | Control/migrator/runner/docs images build and run verified with the Docker daemon; Compose clean install verified to migrate and serve. Agent runtime and validation images, hosted GitOps reference, restore drill not done. |
| T16 | partial | Agent qualification against the exact binding plus capability endpoint and GUI panel. `agy` is now a distinct accepted provider (separate from Gemini CLI) in connection validation, support matrix and connection form. Runtime custody/factory and managed login/logout still not wired; no live entitlement evidence. |
| T17 | partial | Agent qualification flow in Connections. Forge/model/runner qualification flows largely pre-existed; SaaS/OSS qualification matrix still incomplete. |
| T29 | partial | Overview, Runs, Changes, Deployments, Usage, Policies, Runners and Organisation work surfaces plus a route-surface regression gate (one title, one primary toolbar, 390px no-overflow, axe on admin routes) and visual captures with route/runtime metadata. Connections/Campaigns/Findings consistency and reviewed baselines for populated empty/error/blocked/stale states remain. |
| T31 | implemented | Approved custom command profiles, protocol executor and real container test. Controller-to-runner dispatch and repair/model-route selection are still not wired; profiles are managed and gated but not yet invoked by the repair engine. |
| T27/T28 | not started | Qualification and release handoff outstanding. |

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

Not run: hosted sandbox isolation, live provider certification, full non-development
Compose install (no OIDC/HTTPS origin supplied).

## Running the local stack

Development server (current session, detached):

```sh
set -a; . .local/development.env; set +a
export REFORGE_MODE=development REFORGE_FIXTURE_AUTH=true REFORGE_EDITION=self-hosted \
       REFORGE_ADDRESS=127.0.0.1:8080 REFORGE_PUBLIC_URL=http://127.0.0.1:8080
setsid --fork ./bin/reforge >.local/server-solo.log 2>&1 </dev/null
```

Stop: `pkill -f 'bin/reforge'` (also stops the older `8081` repair fixture process
`1974931`; restart it separately if needed). PostgreSQL runs from `/tmp/reforge-postgres`
on `127.0.0.1:55432` (pid was `1969990`).

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
- T31 profiles are managed and gated but the repair engine does not yet select a profile
  as a model/agent route; the controller-to-runner dispatch is the remaining slice.
- T16 runtime custody/factory and managed login/logout are not wired; no live account
  evidence exists, so every official runtime stays disabled.
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
`310d550`, `5dd6b5b`, `d6b0117`, `fb9f081`, `9e52ea2`, `6643005`. The T25 commit is large and includes inherited
uncommitted work; review the campaign authority callbacks in
`internal/campaign/authority.go` and `execution.go` first.
For T31 review `internal/customcmd/{executor,rules,service}.go` and the container test;
confirm the sandbox stdin addition in `internal/sandbox/runtime_linux.go` is bounded. For
T16 review `internal/agent/qualification.go` and `internal/httpapi/agentqualification.go`,
especially the binding derived from the connection rather than the request body, plus the
new `agy` provider validation in `internal/connections/service.go`.
For T29 review `web/tests/route-surface.spec.ts`, `web/tests/visual.spec.ts`,
`web/src/components/DataTable.tsx`, the topbar rules in `web/src/styles/app.css`, and the
removed headings in `web/src/app/{UsagePage,PoliciesPage,RunnersPage,CampaignsPage,CustomProfilesPanel}.tsx`.

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
| T16 | partial | `agy` distinct provider added (re-run unit/integration pass) | Runtime custody/factory and managed login/logout not wired; official accounts/entitlement evidence |
| T17 | partial | connection/agent qualification GUI | SaaS/OSS qualification matrix; live probes |
| T18 | local complete (inherited) | bot ownership/dedup | — |
| T19 | local complete (inherited) | repair loop, trusted validation | Larger model for upgrade fixture |
| T20 | local complete (inherited) | findings/runs/changes GUI | — |
| T21 | local complete (inherited) | merge controller, native proof | Live provider certification |
| T22 | local complete (inherited) | CI/CD orchestration | Live native approvals |
| T23 | local complete (inherited) | GitOps promotion | Hosted GitOps reference (T26) |
| T24 | local complete (inherited) | policy/deployment/usage/audit GUI | — |
| T25 | local complete (inherited) | campaigns, fairness | External native-provider campaign run |
| T26 | partial | control/migrator/runner/docs images, Compose install (inherited) | Agent runtime and validation images, hosted GitOps reference, restore drill, full installation acceptance |
| T27 | not started | isolation/concurrency/recovery qualification | Depends on T26/T29/T30/T31 completion |
| T28 | not started | release qualification and operator handoff | Depends on all prior |
| T29 | partial | route-surface gate, help-link test, visual metadata (re-run pass) | Reviewed populated/empty/error/blocked/stale baselines per family; Connections copy pass |
| T30 | local complete (inherited) | MkDocs `--strict`, docs container, help links (re-run route help-link test) | — |
| T31 | implemented | profiles/executor/container test (container test re-run pass) | Controller-to-runner dispatch and repair/model-route selection |

Validation matrix: V01–V05, V07–V18, V20–V22 inherited local evidence; V06 (official
agent runtime) and V19 (clean install/restore) partial; V23/V24/V26 partially re-run here;
V25/V27 not satisfied. G5 remains open.

### Stopping condition

**Interrupted, not complete.** Neither the normal-finish nor externally-blocked condition
is met: T16, T26, T27, T28, T29 and T31 retain locally implementable work that this
session did not finish, and no single missing external credential is the sole blocker.
The next executable actions, in dependency order:

1. T31: add a versioned profile binding to the workflow task/repair input, extend
   `repair.ExecutionContext` with the approved profile spec, and add a runner-side
   custom-profile branch that calls a controller authorize endpoint (durable budget
   reservation, fresh policy/profile/digest fencing) then executes via
   `customcmd.Executor` and reports usage. Reuse `customcmd.Service.Bind` for the
   authorization transaction. Add OpenAPI + generated types and a controller/runner
   integration test.
2. T16: implement a runtime custody factory (`agent.RuntimeFactory`) that builds the
   Codex bridge from a qualified connection and an isolated runtime namespace, plus
   managed login/logout/status HTTP routes and GUI controls, disabled with a reason until
   custody is configured. Test with the existing JSONL fixtures.
3. T29: capture and review populated/empty/error/blocked/stale baselines per route family
   and finish the Connections explanatory-copy pass.
4. T26: agent-runtime and validation image references, hosted GitOps reference manifests,
   and a scripted encrypted-backup restore drill; then T27/T28 integrated qualification.

No `.cc-writes`/`.claude` cleanup was needed at this checkpoint; `/home/mnorris/repos/.claude`
and all non-empty directories were preserved.
