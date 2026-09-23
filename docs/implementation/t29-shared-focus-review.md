# T29 shared focus and responsive review

Date: 2026-09-23. Local review only; no commit.

## Result

SplitView no longer moves focus to an empty list on cold mount under React StrictMode. It tracks prior selection and restores focus only on a real detail close; focus returns to the opener when it remains mounted, otherwise to the list container. Keyboard close, browser Back, deep-link close, and 1440/390 cold-load behavior pass in `web/tests/split-view-focus.spec.ts`.

The invitation acceptance report's `navOverlayAfterResize` flag is a measurement artifact: it reads `getBoundingClientRect().right > 0` immediately after `setViewportSize`, while the sidebar has a 180 ms CSS transform transition. A new browser check waits for settled off-canvas geometry and confirms no scrim or expanded menu after desktop-to-390 resize. It passes; no AppShell fix was needed. The existing `/invite` AppShell bypass remains preserved.

Dead legacy selectors removed from `app.css`: `.audit-filterline .audit-advanced`, `.audit-advanced .form-grid`, and generic `.audit-export` padding/border. Audit page uses `.audit-advanced-fields`; export spacing is defined by the more specific `.audit-results-bar .audit-export` rule in `audit.css`. Existing route-specific audit behavior remains. Split-list programmatic focus no longer gets a persistent focus outline (`:focus-visible`); detail focus outline and responsive split layout remain. Removed duplicate mobile `.back-link` display rule already supplied by its base rule.

## Checks

- `tests/split-view-focus.spec.ts` — 3 passed at 1440/390 and deep-link close (included in lifecycle run).
- `tests/shell.spec.ts` resize regression — passed; settled sidebar bounds are off-canvas, scrim absent, Menu remains collapsed.
- `npm run typecheck` — passed.
- Shared route scan: 52 light/dark desktop/mobile route records plus two skip-link states, zero axe violations and overflow; 3/3 route-surface tests passed.

Confidence: 96% for local focus and resize behavior. Full production-built browser suite awaits coordinator service setup.
