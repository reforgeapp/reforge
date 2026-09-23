# T28a/T29 current browser acceptance

Run date: 2026-09-23. Source: `35b7545db32643047163412a754d29379e1ffc1c` (`feat(gui): show active organisation login`). Build and browser suite ran from detached worktree `/tmp/reforge-t28a-current`; main worktree changes were not included.

## Setup

- Built with `npm --prefix web run build` and `go build -trimpath -o /tmp/reforge-t28a-current-server ./cmd/server`.
- Served Go-built web assets on `127.0.0.1:8094` against fresh PostgreSQL 18.6 on `127.0.0.1:55435`, database `reforge`.
- Applied committed migrations with separate migration role. App used `reforge_runtime`; verified `rolsuper=false`, `rolbypassrls=false`.
- Fixture authentication enabled. Fresh database seeded only Development organisation; no imported repositories or provider credentials. No live forge, model, agent, OIDC issuer, paid API or customer repository used.
- Playwright 1.63.0, Chromium 153.0.8010.12.

## Results

`npm --prefix web run test:e2e -- --workers=2`: **153 passed, 27 failed, 15 skipped** of 195 tests (5.8 minutes). This is not a passing GUI gate.

The 27 failures group as follows:

| Area | Failures | First failing expectation |
| --- | ---: | --- |
| Connections lifecycle | 3 | Error state, filtered empty state and “Load more connections” were not found in the route-mocked flows (`admin-data-lifecycle.spec.ts`). |
| Merge keyboard | 1 | “Preview merge gate” did not receive focus at 390px (`changes.spec.ts:41`). |
| Findings detail | 1 | Finding row action was absent (`findings-close.spec.ts:44`). |
| Audit and policy editor | 3 | Audit event row action, advanced JSON section and “Open simulation” control were absent (`insights.spec.ts`). |
| Policy impact | 10 | Each scenario timed out waiting for “Open simulation” (`policy-impact.spec.ts`). Current page presents simulation through the policy editor tabs; test/page contract needs review. |
| Route accessibility | 1 | Audit page has duplicate `region` labels: `.split-list` and `.table-card` both use `aria-label="Audit events"`. |
| Team cache | 2 | Expected “Teams” heading and versioned team request were absent in mocked lifecycle flows (`team-cache-lifecycle.spec.ts`). |
| Organisation switch | 1 | After Enter on “More…”, the management dialog remains open and intercepts the next click on “Switch organisation”. |
| Dark theme | 3 | Two checks expected repository table headers in a fresh empty database; one dark route scan found the same duplicate Audit landmarks. |
| Workspace navigation | 2 | One test targets absent org `00000000-0000-4000-8000-0000000000de`; another checks mobile navigation without establishing the mocked session and found no `#primary-navigation`. |

Locator and focus failures need isolated follow-up before attribution. Full names, first assertions and Playwright output are in `../../.local/t28a-current-browser/full-suite.log`; targeted traces are in `../../.local/t28a-current-browser/failure-traces/`.

One independent shell test run reproduced the dialog interception at `shell.spec.ts:199`: menu items were `Development` and `More…`; Enter on the latter opened the management dialog. Trace, error context and screenshot are retained under `../../.local/t28a-current-browser/failure-traces/shell-dialog/`.

## Additional route and visual checks

- Direct cold load, sidebar client navigation away/back, then browser reload passed for all 13 workspace/admin routes. Each stage retained one matching page heading and returned HTTP 200. Results: `../../.local/t28a-current-browser/captures/route-recovery.json`.
- All 13 routes captured at 1440×900 and 390×844 in light theme; each had one page heading, at most one primary toolbar, and screenshots for actionable API failure states. Metadata: `../../.local/t28a-current-browser/captures/metadata.json`.
- All 13 routes captured in dark theme at desktop and 390px. Settled mobile checks found no horizontal overflow and no open navigation drawer across all routes. Measurements: `../../.local/t28a-current-browser/captures/dark-mobile-layout.json`.
- Blocked-run and stale-repository visual fixtures passed 2/2. These are presentation fixtures, not persisted-provider evidence.
- Theme keyboard toggle and mobile drawer keyboard tests passed in the full run. Organisation-switch keyboard sequence is blocked by the open modal described above.
- Visual captures are review artifacts only; no human visual signoff was performed.

## Default opt-in skips

The full default run skipped 15 tests. Three visual captures were then explicitly enabled and passed: one route-baseline test plus two blocked/stale-state tests. Remaining 12 were not certified:

- 5 connected GUI tests require isolated acceptance backends on `:8090` or `:8091` and disposable Gitea for reviewer acceptance.
- 2 live forge tests require disposable Gitea and enrolled runner (`connections.spec.ts`, `findings.spec.ts`).
- 1 repair test requires disposable Gitea, Ollama, runner and gVisor.
- 1 connection mobile test requires `REFORGE_CONNECTIONS_MOBILE_URL` fixture app.
- 1 install OIDC test requires a non-development origin and disposable issuer.
- 2 database-backed campaign/controls tests hard-require disposable database `reforge_dev` on `127.0.0.1:55432`; this run deliberately used isolated `reforge` on `:55435`.

No external provider certification follows from these fixture runs. T28/G5 remains open.

## Artifacts and cleanup

- Logs, 65+ route/theme/state captures, measurements, traces and screenshots: `/home/mnorris/repos/reforge/.local/t28a-current-browser/` (ignored local artifacts).
- First browser attempt is retained as `full-suite-setup-failure.log`, excluded from counts: I stopped its server while cancelling an incorrect readiness loop; its 180 connection-refused errors are harness failure.
- Detached worktree `/tmp/reforge-t28a-current`, server on `:8094`, and PostgreSQL container on `:55435` were used only for this run. Leave server available until coordinator confirms follow-up review is complete; then remove only these owned resources.
