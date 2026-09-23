# T28a original opt-in browser review

Run date 2026-09-24. Scope: the 15 opt-in scenarios skipped by the `675b0fd` production-browser suite (193 passed / 15 skipped). Local fixture-auth evidence only. No provider certification or G5 claim.

## Build and isolation

- Source: clean worktree `/tmp/reforge-production-browser-675b0fd`, HEAD `675b0fdc0bb315c44506dc417cfa1f686d116a4a`. No `web/`, `internal/` or `cmd/` diff between `675b0fd` and current `HEAD`. Specs were run from that worktree, byte-identical to `675b0fd`.
- Server: `.local/opus-resume/production-browser/reforge-server-final`, SHA-256 `fe052c4455eb8f53133aba18219fb3e57650de6b02523a0c1104d3bd5508a478` (same binary as the production-browser run). Each phase compared served bytes to `web/dist`: `index-D6b9Alkr.js` `83b9348b…91dc1` and `index-kbiuJPQ9.css` `c15f147c…36ca6e` matched (`logs/served-*.txt`).
- Isolation: user plus network namespace (`run.sh`) with loopback only and no external network. It contains its own PostgreSQL 18.6 cluster (`/tmp/reforge-postgres/bin`, data under `optin-browser/pgdata`, bound to namespace `127.0.0.1:55432`, `shared_buffers=64MB`, `max_connections=40`, SCRAM auth), the Go server on namespace `8090`/`8091`/`8092`, and Chromium 153. The run bound no host ports, so there was no conflict with the repair worker's host `55432`, demo `8084`, docs `8082` or PG `55437`.
- Roles: `reforge_migrator` (NOSUPERUSER, BYPASSRLS, migration/seed only) and `reforge_runtime` (NOSUPERUSER, NOBYPASSRLS; server and spec SQL). Databases: `reforge_optin` for connected/admin/mobile/findings/visual. A separate disposable `reforge_dev` exists **inside the namespace only** because the insights/campaigns specs hard-guard `/reforge_dev` on port `55432`. The demo `reforge_dev` on host `55437` and `/reforge_test` were never touched.
- Seed data (`inner.sh` `seed_optin`, migrator role): Acceptance org `…0002` with owner membership for the fixture user, a Gitea forge row `…0003` with an `.invalid` endpoint, and repository `…0004`. It also adds one Development-org Gitea row and repository `…0006` for findings import. These are DB rows only; no Gitea import took place.
- Teardown: servers and PostgreSQL stopped when each namespace run exited. `pgdata` was then deleted. The demo remained healthy (`/readyz` 200, docs 200).

Command: `bash .local/opus-resume/optin-browser/run.sh a b c pa pc`. Final clean run 2026-09-24T08:37:39+10:00 → 08:41:34, one worker. Stability check: `REPEAT=3 run.sh pa pc`.

## Results

"Original" = unmodified `675b0fd` spec. "Patched" = copy in `optin-browser/patched/tests/` with selector updates only for the current UI (`artifacts/patched-spec-changes.diff`). Patched passes are evidence of backend/GUI behavior. They are not passes of the repository specs.

| Spec / test | Original | Patched (×3 repeat) | Evidence type | Failure cause |
| --- | --- | --- | --- | --- |
| connected-admin-acceptance: runner pool + policy persistence, org isolation | **pass** | n/a | Real GUI → Go → PG; no interception | — |
| connected-admin-acceptance: 390px dialogs light/dark | **pass** | n/a | Real backend session | — |
| connected-acceptance: team CRUD, org scope, disabled Codex route | fail | pass | Real GUI → Go → PG; no interception | Stale: Teams is now a tab; creation uses the `Create team` dialog; teams list renders as buttons in `navigation "Teams"`, not rows |
| connected-acceptance: 390px keyboard light/dark | **pass** | pass | Real backend session | — |
| insights-live: pause policy → simulate → activate, budget, audit NDJSON export | fail | pass | Real GUI → Go → PG; spec SQL seeds org/repo via runtime role | Stale: Reason moved to Review tab; audit action renders as `Open Policy Changed event` button |
| campaigns-live: planned → canary → paused → canary → cancelled, budget link | fail | pass | Real GUI → Go → PG; policy set up through API; no dispatch | Stale: `Plan campaign` opens a 3-step planner; member JSON sits under a collapsed `<details>`; reload restores selected detail; `Scope ID` is now the `Named scope` select |
| findings live: import advisory → assign/unassign/dismiss/reopen/snooze | fail | pass | Real GUI → Go → PG; repository row SQL-seeded, not imported | Stale: actions moved to Repair tab. Unassign is disabled until a reason is entered (see defects) |
| connections-mobile: 390/1440 layout, persisted connection, keyboard detail | fail | pass | Real GUI → Go → PG; DOM text probe | Stale: mobile layout is now cards, so `custom_command` fits on one line (`lines > 1` fails while containment holds). Close label is now `Close connection details` |
| visual-states: sign-in/finding detail | pass | n/a | **Fixture only**: `page.route` meta/session/findings | — |
| visual-states: blocked run/stale repository | pass | n/a | **Fixture only**: `page.route` tasks/repositories | — |
| visual: 13 routes × 1440/390 + error states | pass | n/a | Real backend for route captures. Error states are **fixture** (`route.abort`) | — |

Capture sets only. No baseline or comparison claim. 39 route/state captures plus `metadata.json` in `artifacts/visual/`. Connected/admin captures and request logs are in `artifacts/connected-gui/` and `artifacts/connected-admin/`; mobile widths/captures are in `artifacts/connections-mobile-{original,patched}/`.

Not run (4 of 15):
- `install-oidc.spec.ts`: signed local OIDC identity was accepted separately.
- `repair-live.spec.ts`, `connections.spec.ts` live Gitea lifecycle, `connected-reviewer.spec.ts`: need Gitea `53000` / Ollama / runner, reserved for the repair worker.

## Defects found (outside ownership, proposed fixes)

1. **Mobile sidebar shadow bleeds while closed.** In `web/src/styles/app.css:279` (≤720px), `.sidebar` keeps `box-shadow: 0 14px 38px …` while at `translateX(-104%)`. About 28px of shadow shows on the left edge of every 390px page (`artifacts/visual/overview-390.png`, `connections-mobile-*/connections-after-390.png`). Fix: move the `box-shadow` declaration to `.sidebar.sidebar-open`.
2. **Findings Unassign requires an unused reason.** In `web/src/app/FindingsPage.tsx` the Unassign button is disabled on `!reason.trim()`, but `clearAssignment` sends `{ action: 'assign', assigned_to: '' }` with no reason. Fix: drop `|| !reason.trim()` from the Unassign button. The alternative is to send and record the reason.
3. **Stale opt-in specs.** Five repository specs fail against `675b0fd` because of UI drift. The minimal selector fixes are in `artifacts/patched-spec-changes.diff`. The diff also adds a post-reload wait in campaigns: an immediate `isVisible()` raced render and clicked a hidden list button, which has no action timeout and hung until test timeout.
4. **Spec hygiene.**
   - `connected-acceptance.spec.ts` test 1 writes `backend-requests.json` without `mkdir`; the directory is only created by test 2.
   - `connections-mobile.spec.ts` writes to a fixed repo `.local/t29-mobile-connections/` path, which overwrites T29 evidence. The worktree output was relocated here.
   - `insights-live`/`campaigns-live` hardcode port `55432`, database name `reforge_dev` and `/tmp/reforge-postgres/bin/psql`. That guard conflicts with shared host port use; it was satisfied here only through namespace isolation.

## Limits

- Fixture auth and an SQL-seeded second org/repositories are not production identity or forge-import evidence.
- The `.invalid` Codex/Gitea records prove disabled/unverified states only.
- Campaigns ran without dispatch authority.
- Visual captures are single-run, not comparative (V29).
- Some policies-390 captures were taken before version history loaded; that is a capture-timing artifact, not a claimed defect.
- Diagnostic attempt logs are kept under `optin-browser/attempts/`.
