# T29 visual comparison

Status: matched local captures reviewed; human baseline approval remains open.

## Capture setup

- Baseline web: `b00e92930664900e6fb5ac5fbdf358d402ffbe0d`, genuine pre-route-rebuild revision.
- Current web: `b25a2d0e891080ee8c639b320f3e41adf13abb97` at capture review. Its web source matches the bundle built at `567996f826d2b17e512c41463d7272a837de722f`; later commit changed tests/docs only. Current Go backend binary was built at `567996f` and serves both frontends.
- Topology: two loopback-only Go servers, baseline assets at `:8094`, current assets at `:8095`; one disposable PostgreSQL 18 Alpine database on `:55441`. Same persisted fixture user, Demo Operations organisation (`00000000-0000-4000-8000-0000000000de`), six repositories, four findings, and four unverified connections served both. App role was non-owner/non-superuser with `NOBYPASSRLS`; only migration and fixture seeding used the database owner.
- Browser: Chromium `153.0.8010.12`, light theme, fresh browser context per revision and viewport, service workers blocked. Login used explicit fixture auth. After login, navigation went directly to Demo Operations Overview because the empty Development org breaks the baseline Overview; then Connections was opened via the visible link on desktop or Menu → Connections on mobile.
- Both routes made real requests to the current Go backend. Across the four matched contexts, all API responses were 200, with zero API errors and zero browser exceptions. No request interception or fixture API replacement was used. Asset URLs and SHA-256, exact steps, and rendered text are in [matched metadata](../../.local/t29-visual-comparison/artifacts/matched-metadata.json).
- Both Go servers and the disposable database were stopped and removed after capture. Existing demo services were not touched.

## Captures

All captures use the first viewport, not full-page screenshots.

| Route | Baseline 1440×900 | Current 1440×900 | Baseline 390×844 | Current 390×844 |
| --- | --- | --- | --- | --- |
| Overview | [PNG](../../.local/t29-visual-comparison/artifacts/baseline-overview-1440.png) | [PNG](../../.local/t29-visual-comparison/artifacts/current-overview-1440.png) | [PNG](../../.local/t29-visual-comparison/artifacts/baseline-overview-390.png) | [PNG](../../.local/t29-visual-comparison/artifacts/current-overview-390.png) |
| Connections | [PNG](../../.local/t29-visual-comparison/artifacts/baseline-connections-1440.png) | [PNG](../../.local/t29-visual-comparison/artifacts/current-connections-1440.png) | [PNG](../../.local/t29-visual-comparison/artifacts/baseline-connections-390.png) | [PNG](../../.local/t29-visual-comparison/artifacts/current-connections-390.png) |

Baseline assets: `/assets/index-D3RPwlen.js` SHA-256 `692e5ab689dfa1c217512c9c91c917fa35364c7335506b84a19bae58dc2162d8`; `/assets/index-y7ehPRPO.css` SHA-256 `b6f59bb4a1e2be2191e6e4c070295985d73f1dfad3a33dd13dc5bf05f7d8d0b8`.

Current assets: `/assets/index-Crtub4So.js` SHA-256 `a9f32933dce579f0ec96f87748b021deaccf4de7ac2804c03401053bf83e5adb`; `/assets/index-Cp7V3BTd.css` SHA-256 `c697db88524244cf6e29fba8ffaaa7dd5fb89513a6efd32699de271723c45328`.

## Findings

The Overview rebuild keeps the same five portfolio counts, capacity figures, four findings, and repository data. It removes the stack of large bordered panels and presents the work queue and portfolio as tabs. At desktop size this reduces visual weight while keeping the same key information. At 390px, the current two-column metrics fit, but the work table extends horizontally beyond the first viewport; reviewers must scroll the table to see remaining columns.

Connections changes from one mixed table followed by a large profile form to provider tabs, search/state filters, and a list/detail workspace. This better separates forge, model, agent, and delivery records. The old default shows all four connection kinds; current default selects Forges and shows the same Gitea record, with the other records under their tabs. At desktop width, the unselected detail panel is hidden and the list uses the full width. At 390px, both filter inputs fit; table columns clip at the viewport and require contained horizontal scrolling to reach state and actions. Keep the filters as-is; mobile table access is the remaining layout finding.

The comparison uses the current Go API for both web revisions, not the old Go backend. The populated Demo Operations routes work without errors, but the baseline web has a separate empty-state defect: its default Development Overview crashes when current API returns `portfolio: null` and baseline source calls `value.portfolio.map`. The matched run navigated directly to the populated org, whose `portfolio` is an array. This caveat limits compatibility claims to the captured populated routes.

This is local visual evidence for two routes and one theme. It does not establish every route/state/theme, human visual approval, G5, or external certification.

Confidence: 95% in capture identity, shared data, and visual observations; human product approval remains outstanding.
