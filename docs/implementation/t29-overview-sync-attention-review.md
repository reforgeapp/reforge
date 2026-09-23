# T29 Overview sync attention

Date: 2026-09-23

The Overview sync count is now an amber status link with a warning mark and direct “Review stale repositories” action. Singular/plural wording is preserved; no row appears at zero. The target remains the Repositories `status=stale` filter.

## Verification

- `npm run typecheck -- --pretty false` — passed.
- `npm run build` — passed; Vite reported its existing large-chunk advisory.
- `tests/overview.spec.ts` — 5 passed. Checks action URL, keyboard focus, singular/zero counts, theme color change, 390px fit and axe results.
- Before/after captures use same Playwright browser fixture at 1440×900 and a 390×844 mobile viewport, light/dark (full-page mobile image height is 942px): `.local/t29-overview-sync-attention/{before,after}/`.

The browser suite intercepts Overview API data. It verifies warning presentation and destination URL; it does not verify backend stale-count or repository filtering behavior.
