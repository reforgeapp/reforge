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
| final HEAD | `e0713ee` (see `git log`) |

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
| T29 | partial | Header gate, Overview and Organisation connected. Remaining routes still render their pre-existing work surfaces; route-coverage and visual-regression baselines for every family are not complete. |
| T30 | local complete (content) | MkDocs `--strict` build passes locally; docs container image build unverified (no daemon). |
| T26 | partial | Control/migrator/runner/docs images and Compose authored. Agent runtime and validation images, hosted GitOps reference, restore drill not done. |
| T16 | not started | Codex bridge exists from prior work; production custody/wiring and entitlement qualification outstanding. |
| T17 | not started | Qualification GUI flows outstanding. |
| T31 | not started | Custom command runtime not implemented; route stays disabled and documented. |
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
- `go test -race -count=1 -timeout 25m ./test/integration/...` — pass `58.956s` against
  disposable PostgreSQL 18.6 (migration applied first).
- `go test -count=1 ./...` — pass.
- Campaign integration subset `Campaign|DeploymentContinue` — pass `10.403s`.
- Browser suite `npx playwright test --grep-invert live` against `127.0.0.1:8080` —
  `87 passed, 1 skipped`. New specs: `web/tests/overview.spec.ts`,
  `web/tests/organisation.spec.ts`.
- `mkdocs build --strict` (material 9.7.0, local deps in `/tmp/mkdocs-deps`) — pass,
  `search/search_index.json` generated.
- Live endpoint probe: `/api/v1/orgs/00000000-0000-4000-8000-000000000001/overview`
  returned real scoped counts after fixture sign-in.

Not run: container image builds (`docker`/`podman` daemon unavailable), hosted sandbox
isolation, live provider certification, T31 container protocol test.

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
- T31 custom command runtime is unimplemented; the GUI must keep it disabled.
- T16/T17 production agent custody and qualification GUI are outstanding.
- Container images are unverified because no daemon is available.
- `web/tests/merge-settings.spec.ts` and `web/tests/deployments.spec.ts` had brittle
  locators from earlier route additions; locators were tightened. `merge-settings`
  non-Gitea now waits for the primary connection provider before saving.

## External actions required

- Docker/Podman daemon to build and run the Compose stack, then verify clean install,
  migration, restart, docs serving and no-Docker-socket.
- Sandbox host with working cgroup delegation for hosted untrusted execution.
- Live GitHub/GitLab test organisations and credentials for provider certification.
- Official agent accounts only where terms and topology permit; Codex/Claude/`agy`
  qualification evidence.
- Paid model test budgets if direct-API certification is required.

## Review boundary

Review `7815b0c`, `9215b9f`, `32c365b`, `3306efd`, `e0713ee`. The T25 commit is large and
includes inherited uncommitted work; review the campaign authority callbacks in
`internal/campaign/authority.go` and `execution.go` first, then the header/help changes in
`web/src/app/SectionPage.tsx` and `web/src/components/Help.tsx`.
