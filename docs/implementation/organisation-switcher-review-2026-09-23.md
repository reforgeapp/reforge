# Organisation switcher follow-up — 2026-09-23

T29j reopened from owner screenshots after the prior GUI checkpoint. One Luna worker
implemented; Astra reviewed. No backend, tenant authorisation or session/cache logic changed.

## Findings

- Menu positioned at a fixed 250px from the header, roughly 98px right of its trigger.
- Menu overflowed horizontally in light/dark at desktop and 390px.
- Open menu failed axe `scrollable-region-focusable`; earlier closed-route checks missed it.
- Every row repeated Active; More scrolled out with the list.

## Repair

Borderless inline trigger; desktop name/chevron and compact mobile initial/chevron.
Menu anchored to the trigger on desktop and contained below the header on narrow screens.
Single-line names, current check, Paused only when relevant. Full names remain available
through titles. One scrollable, keyboard-focusable list; More remains outside its scroll area.
Existing organisation records, tenant cache clearing and More modal retained.

Review additionally caught focusable-group arrow navigation and mobile trigger/search overlap;
those corrections belong to this same bounded follow-up. No new menu positioning dependency.

## Verification

Artifacts: `.local/org-switcher-2026-09-23/`. `before/` records the original failure;
`after/` is an intermediate capture. `delivered/` contains current closed/open screenshots
and `review.json`: light/dark at 1440×900 and 390×900, zero axe violations, horizontal or
page overflow, and successful Escape/focus restoration in all four cases. Desktop menu
and trigger share the same x coordinate; narrow menu stays inside viewport bounds.

`tests.json`: **17 passed, 0 failed, 0 skipped** (shell and theme suites). Tests cover long
unbroken names, current selection, list scroll, visible footer before/after scrolling,
arrow navigation from rows and list group, Tab/Shift+Tab/Escape, outside interaction,
More modal, theme persistence and mobile search/trigger geometry. `build.log` passed.
The earlier 133-test integrated suite remains historical; this scoped change received
its affected suites and real-browser visual/accessibility checks.

Delivered assets: `index-DSHwFx-h.js`, `index-DYviOhxY.css`.
Demo: `http://127.0.0.1:8080` (existing app/DB/docs stack retained).


Control image `reforge:gui-review` rebuilt (`container-build.log`):
`sha256:0117be5da59ea0736d73f9a3ec0a9b1e865616db06afae0bf7d19ce52f1ae9f7`.
Packaged assets match the served bundle; non-root uid 10001, binary/migrations and no
Docker socket verified. No other services started. Existing owner edits to
`copilot-handoff-2026-09-22.md` preserved.
