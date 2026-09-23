# Shared controls and search — 2026-09-23

T29p locally complete. Starting HEAD `b13fa40`. Two Luna workers owned CSS/tokens and
search/tests respectively; Astra reviewed the complete diff and browser evidence.

## Changes

- Policy editor/history, detail panels, metrics, tables and grouped forms use spacing and
  dividers instead of nested frames. Editable fields, overlays and status notices retain boundaries.
- Shared inputs/buttons use consistent heights and restrained corners.
- Repository search has a quiet background, one outer focus treatment and a labelled submit
  action. Enter, icon submission, clearing, URL filters and back navigation retain existing behavior.
- `type=search`, autocomplete off, autocorrect off and spellcheck off distinguish repository
  names from general prose. The screenshot's Gitea popup was not rendered by application code;
  browser history is the likely source. Browsers/extensions may override autocomplete preferences.
- At 720px and below, search occupies its own full-width header row. Navigation/menu offsets
  follow the taller header. Organisation switching and cache/session boundaries remain unchanged.
- GUI specification records these rules, including review of open/focused/error states.

Review caught incomplete mobile selectors, inherited input sizing, duplicated policy boundaries
and one stale test locator; Luna corrected them. No new dependency, backend or API change.

## Verification

Artifacts: `.local/shared-controls-2026-09-23/`.

| Evidence | Result |
| --- | --- |
| Production frontend build | TypeScript and Vite passed; existing large-chunk advisory remains |
| `before/review.json`, `after/review.json` | Same 13 routes × two themes × 1440/390px; 52 cases each, no page errors, axe violations or document overflow |
| `after/` detail/impact captures | Loaded finding details, Escape/focus restoration, real six-repository policy simulation; no axe/overflow failures |
| `browser.json` | Full suite with visual opt-in: 136 passed, one failed, six skipped; failure was the former textbox locator after introducing semantic searchbox |
| `navigation.json` | Locator corrected; all three navigation tests passed. Consolidated result: 137 distinct passing tests, six skipped, no unresolved failure |
| `states/` | Sign-in, detail, blocked-run, stale-repository and route error captures; mocked state evidence only |
| `search/review.json` | Eight real-backend cases: light/dark × 1440/768/390/320px. Query returns payments-api; clear restores six repositories. Focus/submit/Escape, menu/sidebar placement and axe checks pass |
| Search geometry | Outer control 36px; mobile text area 317px at 390 and 247px at 320; no inner focus outline/shadow |
| Container build/smoke | Passed; non-root uid 10001, binary/migrations present, no Docker socket, matching bundled assets |

The six opt-ins not rerun are campaign persistence, private Gitea connection lifecycle,
imported finding triage, policy/budget/audit persistence, non-fixture OIDC and full isolated
repair. Their earlier evidence is unchanged. The route audit uses persisted local demo records;
those unverified demo connections do not certify external integrations. No G5 claim.

## Delivered build

Demo remains at `http://127.0.0.1:8080`; refresh the browser. Existing app, PostgreSQL and
docs services retained; no additional demo stack started.

Assets: `index-BNfGV2Wn.js`, `index-Dsnuv0cZ.css`.
Control image `reforge:gui-review`:
`sha256:977ea3b4038abbb2ac3619cb8bbb8a755c935bd83447d3413d99f977aab55578`.
Build log: `.local/shared-controls-2026-09-23/container-build.log`.

No T29p implementation blocker remains. Broader external qualification, hosted isolation and
release/load gates remain as recorded in progress. Owner edits to
`copilot-handoff-2026-09-22.md` were preserved and excluded from this change.
