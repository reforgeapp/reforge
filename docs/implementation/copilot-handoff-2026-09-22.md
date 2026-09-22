# Usage-pause handoff — 2026-09-22

Owner requested a usage pause. Finish no additional backlog automatically. Resume only on owner instruction. Preserve current work and the running local demo.

Use one Luna agent per ticket, no subworkers. Read `PLAN.md`, applicable `AGENTS.md`, this file and the ticket's source. Use caveman-full. No README edits, source comments, external deployment, paid API calls or provider mutations. Preserve tenant/RBAC/CSRF, policy hashes, native approvals and budget controls. Commit one bounded ticket with accurate current metadata. Report actual checks, unresolved work and changed files.

The earlier full GUI checkpoint passed 108 browser tests with nine opt-in skips, Go tests, `make check`, container builds, and 26 route/eight detail captures. Those results predate the latest owner-feedback edits; do not reuse them as proof of the final feedback batch. See `gui-rebuild-review-2026-09-22.md` and `.local/rebuild-final/`.

## Tickets

### T29g — Stored-evidence policy impact preview

Status: unfinished; not approved for integration. Copilot Luna may implement the bounded frontend, but Astra must review before this ticket closes.

Own `web/src/app/PolicyImpactPreview.tsx`, a focused browser spec, and its eventual mount in `PoliciesPage.tsx`. Check `.local/wip/` for parked work before starting. Existing policy presets are separate work; preserve them.

Acceptance: enumerate affected accessible repositories by organisation/team/repository scope with bounded pagination. Preview persisted findings/changes through the real policy simulation API. Use only each record's own head/target and actual effective policy; never invent native approvals, tested revisions or protection evidence. Keep missing/stale evidence unknown. Show actual gate changes and backend blockers in a compact table. Label partial coverage, reject invalid/repeated cursors, bound concurrent reads and discard late results after input/scope/role changes. No activation or provider calls. Test cross-repository bindings, pagination, failures and stale responses. Existing advanced manual simulation must remain functional.

### T29h — Repository inventory disappears after sidebar navigation

Status: cache-clearing fix implemented; broader navigation regression remains under T29n. Suitable for Luna independently if further failures appear.

Own `RepositoriesPage.tsx` and repository browser tests. Likely cause: cached query data populates rows, then mount/filter effect clears rows without a new data identity. A full reload through Overview works while client-side sidebar navigation stays empty.

Acceptance: Overview → Repositories → another route → Repositories retains real inventory with a warm query cache. Search/clear, saved views, pagination, browser back and organisation switching remain correct. Empty state appears only for an actually empty response. Add the reproducer, not a heading-count test.

### T29i — Organisation route crash

Status: fixed; focused regression passed. No implementation remaining unless T29n finds another payload shape.

Own `OrganisationPage.tsx` and `organisation.spec.ts`. Reproduce Development organisation's missing/null membership and team scope arrays. Render valid empty scopes without dereferencing undefined; preserve write payloads and server authority. Verify Teams, Members and Identity against actual local responses and an omitted-array regression.

### T29j — Organisation quick-switch dropdown

Status: dropdown and More/manage modal implemented; keyboard/focus acceptance still needs completion. Suitable for Luna independently.

Own `AppShell.tsx`, relevant styles and shell tests. Current organisation button should open a compact quick-switch dropdown. A More/manage item opens the existing modal. Verify arrow keys, Enter, Escape, outside click, focus restoration and switching tenant query state. Do not use unsupported refs on the shared Button wrapper.

### T29k — Campaign and GitOps layout cleanup

Status: open. Suitable for Luna independently.

Own `CampaignsPage.tsx`, `GitOpsPage.tsx` and narrowly scoped style changes. Campaign state filter and Plan campaign action belong in one compact toolbar. GitOps Configuration should not repeat its active tab as another heading; align environment input and Create draft action. Preserve existing API operations, tabs and disabled reasons. Check desktop and 390px layouts with populated, empty and error states.

### T29l — Findings panel close control

Status: open. Suitable for Luna independently.

Own `FindingsPage.tsx`, `components/Workspace.tsx`, focused styles and navigation tests. Replace Findings' Back to list control with one X at the panel's top right, accessible name `Close finding details`. Restore focus to the originating row; clear selected finding while preserving filters. Other routes retain their current back behavior. Verify keyboard and narrow layouts.

### T29m — Dark mode

Status: toggle, persistence and dark tokens implemented; full route/contrast/narrow-screen review outstanding. Suitable for Luna independently, after T29j because both touch AppShell.

Own `AppShell.tsx`, `styles/app.css`, `styles/tokens.css`, affected route styles and a focused theme test. Persist an explicit light/dark choice; use system preference initially. Catch unavailable localStorage. Theme must cover tables, forms, menus, dialogs, status states and focus, not only the page background. Verify reload, keyboard toggle, contrast and 390px layout. Do not store credentials or tenant selections with theme state.

### T29n — Integrate and verify the feedback batch

Status: open; after remaining T29g–m work. Luna can run checks independently; final security/architecture review remains Astra's role.

Build the actual served bundle, run affected browser tests and then the full browser suite. Exercise Development and Demo Operations with a warm cache, both themes and narrow layouts. Capture exact assets/commit, console errors, screenshots and accessibility findings. Update the handoff/progress and rebuild control/docs images as needed. Do not claim G5 or live-provider certification from fixtures.

## Parallel work

T29h and T29i have separate source ownership. T29k can run separately if its worker leaves shared styles to one designated owner. T29j and T29m must be serial or one worker. T29l touches shared workspace/styles: coordinate that ownership. T29g must not share `PoliciesPage.tsx` with another worker. Maximum three workers, preferably fewer while preserving exclusive files.

## Other release work remains

Existing T16/T26 provider, model, subscription-entitlement and hosted-topology certification remains separate. Supply approved disposable accounts and an explicit API budget before paid/live tests. T27/T28 still require the specified cgroup hostile corpus and real 100-executing-run/50-browser load evidence. Preserve those tickets; this GUI handoff does not close them.

## Local demo

Application: `http://127.0.0.1:8080/org/00000000-0000-4000-8000-0000000000de/overview`. Sign in and choose Demo Operations if needed. Docs: `http://127.0.0.1:8082/docs/`.

Development authentication and unverified demo integrations are intentional. Execution history is not fabricated. Exact start/stop instructions and service identities are in `gui-rebuild-review-2026-09-22.md`.

## Closing worker results

- All three workers stopped at the owner's request; no further implementation queued.
- Shell worker delivered organisation dropdown/More modal, theme toggle and guarded persistence, dark tokens, removal of Overview's lonely inventory button, and the cached-repository clear fix. TypeScript and diff checks passed. Broad browser/theme acceptance remains open.
- Resource worker fixed omitted/null team and membership scope arrays in Organisation display and write payloads. Organisation focused suite: **2 passed**, `.local/org-nullable-final`. Policy preset suite: **8 passed**, `.local/policy-presets-final3`. Build/TypeScript and diff checks passed.
- Policy presets are implemented: Observe, Propose, Merge eligible and Deliver. Explicit draft application preserves limits/requirements/defaults, preserves explicit delivery allowlists, blocks raw-JSON conflicts, and never saves or activates automatically. Final integrated regression still belongs to T29n.
- Workflow worker parked **unintegrated, untested** impact files at `.local/wip/PolicyImpactPreview.tsx` and `.local/wip/policy-impact.spec.ts`. No active import remains. T29k and T29l were not started.
- Root production build passed: `.local/usage-pause-build.log`. Current feedback batch has not received a full-suite run or fresh container rebuild; the earlier container images and screenshot matrix represent the preceding reviewed checkpoint.
- Root real-browser smoke passed: Organisation tabs in Development and Demo Operations, cached sidebar repository revisits, theme persistence across reload, and quick-switch More/modal. Evidence: `.local/usage-pause-smoke.json`. This does not close full keyboard, dark-contrast or narrow-screen acceptance.
- Shell/theme source and `shell.spec.ts` remain uncommitted intentionally for T29j/T29m review. Preserve them; do not reset to HEAD. Reviewed Organisation, policy-preset, repository-cache and Overview changes have separate commits.
- Root's first live preset check created an observe-only demo policy version, then failed an assertion because the server sorts denial arrays canonically. No activation occurred. Do not record that attempted script as passed; validate values without depending on array order when repeating.

Start Copilot with T29k or T29l for the smallest independent fixes. T29j and T29m are implementation-complete but need browser review, especially keyboard behavior and dark contrast. T29g is the larger security-reviewed slice; defer integration until Astra reviews it.
