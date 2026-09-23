# Audit/Policies browser regressions — Opus worker report

Date 2026-09-23. Base HEAD `cd2b94a` + dirty tree. No commits.

## Changes (owned files only)

- `web/src/styles/audit.css`
  - Selected-row activity label (`small`) → `--text-muted`. Axe `color-contrast` failed light 1440 detail state (`--text-subtle` on `--primary-soft`).
  - `.audit-results-bar .audit-export` resets `padding`/`border`. Legacy `app.css:115` `.audit-export { border-bottom }` drew stray rule under "Export page" (1440 + 390).
- `web/src/styles/policies.css`: `.policy-editor details > summary` 12px/650/muted. "Advanced policy JSON" and "Evidence JSON" rendered at UA 16px beside 11–12px labels.
- `web/tests/findings-close.spec.ts`: "other routes keep the back-to-list control" retargeted Audit → Runs. Audit deliberately moved to close-X/no back link in `73cb8ea`, covered by `audit-workspace.spec.ts`. Stale contract, not a product regression.
- Preserved existing dirty edits: `AuditPage.tsx` region rename, `insights.spec.ts`, `policy-impact.spec.ts`. Checked against the real UI: "Advanced policy JSON", Review tab, "Open simulation", "Event data" and "Page N exported" all exist in the current page. These edits fix stale test contracts and do not weaken the tests.

## Diagnosis vs t28a findings

| t28a item | Result |
| --- | --- |
| Audit duplicate `region` labels | Fixed by preserved rename (`Audit results and filters` vs `Audit events`). Probe: no duplicate landmark names in any Audit/Policies state, both themes, 1440/390. |
| Audit row action / advanced JSON / Open simulation absent (insights) | Stale test contract; dirty test edits match current UI. 6/6 pass. |
| Policy impact 10× "Open simulation" | Now reached through Review tab (accepted design). 10/10 pass. |
| findings-close:44 Audit row action | Stale contract (see above). 3/3 pass. |
| Close panel keyboard | Audit: open by Enter → focus on close X; Enter on X → focus restored to opener; Escape from detail closes. Verified in all 4 theme×width combos. |
| Policy Review tab flow | ArrowRight from Scope reaches Review (selected + focused); Open simulation, save, simulate, activate flows pass in fixtures. |
| Dense responsive | No horizontal overflow in any probe state. |

## Checks (exact)

- `REFORGE_BASE_URL=http://127.0.0.1:5173 npx playwright test insights.spec.ts policy-impact.spec.ts findings-close.spec.ts audit-workspace.spec.ts policy-editor.spec.ts --workers=2 --output /tmp/reforge-opus-resume/audit_policy-tests` → initial run 35 passed / 1 failed (findings-close:44). After changes **36/36 passed**.
- `npm run -s typecheck` → exit 0.
- Fixture probe `.local/opus-resume/audit_policy/probe.spec.ts` (custom config `pw.config.ts`; route-mocked session/meta/audit/policy APIs) against a temporary prod build (`vite build --outDir /tmp/...`, `vite preview :5199`, since stopped and removed). It runs axe on the Audit list, Audit detail, Policies Scope and Policies Review + simulation states in light/dark at 1440 and 390, and checks duplicate landmarks, focus and overflow. Before fixes: 1 axe violation (contrast). After: **0 violations, 0 duplicate landmarks, 0 overflow, 8/8 passed**. Results: `probe-prod.json`.
- Screenshots were inspected visually: `.local/opus-resume/audit_policy/shots-prod/` (post-fix, prod) and `shots/` (pre-fix, dev).
- `route-surface.spec.ts` against 5173 → **3 failed**: `page.waitForURL` timeout after `/auth/login`. The Vite proxy targets `:8080`, and there is no Go backend there (`/api/v1/meta` → 502). This is environment absence, not an app result. The route-wide axe gate still needs a backend run. The fixture probe above covers only Audit/Policies.

## Observations outside ownership (requests, not changed)

1. `components/Workspace.tsx` `SplitView`: in dev StrictMode, the double-invoked effect sees `mountedRef` already true, so it focuses `.split-list` on cold load. Audit and Findings show a full-list focus outline on cold load in dev. The prod build does not reproduce this (active element = body). Low priority; the shared owner could use effect-local first-run tracking.
2. `styles/app.css:113–115`: legacy `.audit-advanced summary` / `.audit-export` rules conflict with `audit.css`. Owner of app.css could delete them. The audit.css override covers this meanwhile.
3. Runs detail renders both "Back to list" and its own "Close" button (Runs owner).
4. Skip link visible in full-page Policies 390 captures = capture artifact. It is a fixed element offset by page scroll, while focus was on "Hide inputs". Not reproducible in viewport.
5. Policy tabs wrap to two rows at 390. Readable, so not changed. A scrollable strip is an option if the shared `Tabs` owner wants it.

## Limitations

- Fixture/mocked APIs only. No Go backend, DB or providers. No human visual signoff.
- Full t28a suite was not rerun. Other areas are owned by other workers.

Confidence: 85% for the owned slice. The route-wide axe gate still needs a backend run.
