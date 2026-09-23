# T29t mobile navigation

Status: locally complete.

At widths up to 720px, navigation opens as a modal drawer with a scrim. Escape, scrim clicks and route selection close it. Focus starts at the first section link, stays within the drawer, and returns to Menu after close. Header and page content are inert while open. Crossing to desktop closes the drawer; desktop navigation stays persistent.

Checks: `npm run build`; `npx playwright test tests/mobile-navigation.spec.ts --reporter=line` (4 passed).

Drawer checks wait for x=0 and width=245px before keyboard/outside interactions and captures; every navigation link fits its rendered box without horizontal clipping. Browser evidence: `web/tests/mobile-navigation.spec.ts`; `.local/t29t/navigation-light-390.png`; `.local/t29t/navigation-dark-390.png`; `.local/t29t/playwright-results.json` (4 expected, 0 unexpected, 0 skipped, 0 flaky).

Full integration check: Go-served app `127.0.0.1:8080` served `/assets/index-Crtub4So.js`, matching `web/dist`. Playwright reported 163 discovered, 154 passed, 9 opt-in skips, 0 failures and 0 flaky tests. Report: `.local/t29t/full-suite.json`.
