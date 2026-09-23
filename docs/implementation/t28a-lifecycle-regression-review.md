# T28a lifecycle browser regression review

Date: 2026-09-23. Local review only; no commit.

## Result

Assigned lifecycle/focus suite passes 45/45 against coordinator Vite proxy `http://127.0.0.1:5173` with backend `:8080`, one Playwright worker. `npm run typecheck` passes. Full browser suite waits for coordinator's production-built Go app and matching origin; no suite failure remains in this slice.

Retained shared route scan: `route-surface.log` reports 3/3 tests passed. `routes-axe.json` contains 52 route/theme/viewport records (13 routes × light/dark × 1440/390) plus two skip-link states; all report zero axe violations and zero horizontal overflow. This scan used actual local backend through proxy `:5174`.

## Findings and changes

- Team rename exposed a product cache bug. Organisation mutations invalidated only `['org', orgID, 'teams', 'list']`, while Repositories reads `['org', orgID, 'teams']`; a warmed repository picker retained the old team name. [OrganisationPage.tsx](../../web/src/app/OrganisationPage.tsx#L281) now invalidates the shared `['org', orgID, 'teams']` prefix, which refreshes both finite and paginated query shapes. The regression asserts versioned Save request, persisted rename, and updated picker option in [team-cache-lifecycle.spec.ts](../../web/tests/team-cache-lifecycle.spec.ts#L35).
- Connections refresh/pagination and runner refresh tests use deterministic session and scoped API fixtures. The prior failures came from fixture/service setup; current browser checks pass without changing connection authorization or tenant keys.
- Shell quick-switch fixture now supplies two organisations, so keyboard arrows exercise switching rather than opening “More”. Revocation counts only completed repository requests, avoiding StrictMode-aborted fetches.
- Theme checks provide a populated repository and wait for route content before axe scans. Workspace navigation uses an explicit session and task fixture; detail-close focus remains asserted. Merge keyboard coverage now traverses with Tab/Enter and verifies preview/request results. Protected gate, stale evidence, readonly, tenant and operation-recovery assertions remain intact.
- Pending merge actions remain keyboard-focusable while busy via `aria-disabled`; handlers reject duplicate invocation while busy. Other authorization and gate-disabled conditions remain native disabled controls in [ChangesPage.tsx](../../web/src/app/ChangesPage.tsx#L98).

## Checks

- `REFORGE_BASE_URL=http://127.0.0.1:5173 npx playwright test tests/admin-data-lifecycle.spec.ts tests/team-cache-lifecycle.spec.ts tests/shell.spec.ts tests/theme.spec.ts tests/workspace-navigation.spec.ts tests/changes.spec.ts tests/split-view-focus.spec.ts --workers=1 --output=/tmp/reforge-opus-resume/lifecycle-tests-final2` — 45 passed.
- `npm run typecheck` — passed.
- `node`-backed `route-surface.spec.ts` run recorded in `.local/opus-resume/shared/route-surface.log` — 3 passed; retained route matrix in `.local/opus-resume/shared/routes-axe.json`.

Confidence: 96% for assigned local lifecycle regressions. Production-origin full suite remains pending coordinator setup.
