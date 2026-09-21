# Solo resume handoff

Updated: 2026-09-21. Single implementing agent, no workers, no subagents.

## Checkpoint

- Starting HEAD: `6ef9025 docs: revise product implementation plan`.
- Working tree at start was dirty with uncommitted T25 work (listed below) and did not
  build. `go build ./...` failed with
  `no required module provides package github.com/oapi-codegen/runtime/types`.
- Root cause: `BotRevalidation.task_id` in `api/openapi.yaml` had `format: uuid` without
  the `x-go-type: string` override used by every other UUID field, so codegen emitted
  `openapi_types.UUID` and a runtime import that `go.mod` never required. Fixed at the
  schema; regenerated Go/TS. Also fixed committed gofmt drift in
  `internal/httpapi/discovery.go`.
- Planning revision `6ef9025` was treated as reference only; no reset or checkout.

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
| final HEAD | `a24a8ae` (see `git log`) |

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
| T16 | partial | Persisted qualification against the exact binding plus capability endpoint and GUI panel. Runtime custody/factory and managed login/logout not wired; no live entitlement evidence. |
| T17 | partial | Agent qualification flow in Connections. Forge/model/runner qualification flows largely pre-existed; SaaS/OSS qualification matrix still incomplete. |
| T31 | implemented | Approved custom command profiles, protocol executor and real container test. Controller-side run dispatch into a repair/model turn is not wired; profiles are managed and gated but not yet invoked by the repair engine. |
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

- T29 is incomplete: the product-rebuild route table is not fully rebuilt and no visual
  regression baselines with route/runtime metadata are stored.
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
`310d550`. The T25 commit is large and includes inherited uncommitted work; review the
campaign authority callbacks in `internal/campaign/authority.go` and `execution.go` first.
For T31 review `internal/customcmd/{executor,rules,service}.go` and the container test;
confirm the sandbox stdin addition in `internal/sandbox/runtime_linux.go` is bounded. For
T16 review `internal/agent/qualification.go` and `internal/httpapi/agentqualification.go`,
especially the binding derived from the connection rather than the request body.
