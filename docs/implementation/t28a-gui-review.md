# T28a GUI acceptance review

Reviewed 2026-09-23 against the Go server and PostgreSQL-backed local development environment at `http://127.0.0.1:8080`. Fixture authentication was enabled. Direct browser requests were not intercepted; initial unauthenticated session requests returned expected 401, then sign-in and protected reads returned 200. Write-flow used only the local fixture organisation. No customer repository, provider, paid API or repair queue was touched.

## Build reviewed

- Git revision: `8d86ab2bde83988c0f1b291d5da2391e755701f4`
- Served JavaScript: `index-CJN-NXVT.js`, SHA-256 `a0cfa8b5498243050b77aef60a4f2882988a6da7c99ef1ed0b61789c578367b9`
- Served CSS: `index-Bd1oeqz3.css`, SHA-256 `57d8f580a6a1e1ca93749026d0cb102b63d0cd9e972142c5c87911c0dfc0ae37`
- Downloaded assets matched `web/dist` byte-for-byte.
- Browser: Playwright Chromium, Node `v26.7.0`.

## Connected browser review

The browser signed in through the local fixture identity provider, opened Overview, then read Repositories, Connections, Runners, Policies and Organisation. The local API returned persisted data for each route; no failed data loads or page errors appeared. The browser opened persisted finding `Browser advisory 1789904431708` and inspected Repair readiness. Backend returned HTTP 200 for finding detail, recipes, model/agent connections, custom profiles and runner pools. No repair preview or write was submitted.

Repair preview was blocked because no operator image is configured. The GUI says “Configure a registered operator image before preview” and disables “Generate preview”. A persisted Codex subscription connection was disabled and showed why: qualify an official runtime, account entitlement, topology and budget controls. Both states remain unavailable pending evidence.

The GUI write/reload flow created a disposable team named `t28a-review-1790133845245` in fixture organisation `00000000-0000-4000-8000-000000000001`. `PUT /teams/{id}` returned 200; a persisted `GET /teams` found it at version 1. The browser navigated to Overview, returned to Organisation, and found the team still present. The GUI then deleted it (`DELETE /teams/{id}` returned 204); a fresh persisted list confirmed absence. No fixture data remains from this flow. Raw evidence: `.local/t28a-gui/write-reload-cleanup.json`.

## Narrow layout and keyboard

At 390px with mobile navigation closed, Overview, Repositories, Findings with Repair open, Connections, Policies and Organisation fit the viewport without document/body horizontal overflow. Screenshot `.local/t28a-gui/finding-repair-390.png` shows nav-closed Repair; `.local/t28a-gui/finding-repair-1440.png` shows desktop.

The Connections dialog retained focus inside the dialog, fit the viewport and closed with Escape. Shell keyboard checks covered search submission, tab focus, navigation and browser history. Mobile navigation closes on Escape and on route-link activation; Escape restores focus to Menu and closed sidebar is inert.

**P2 defect — mobile navigation overlay does not dismiss outside and allows focus behind it.** Reproduce at 390px: open a finding’s Repair tab, click Menu, wait for sidebar to finish sliding in, then click outside at `(380, 160)`. Drawer remains open (`aria-expanded=true`); no scrim exists. With drawer open, focus last sidebar link and press Tab: focus moves through main content and reaches “Close finding details” while drawer remains open. Main stays interactive. Navigation by sidebar link and Escape do close it. Treat the open sidebar as an overlay state; it has no backdrop/dismiss layer. Evidence: `.local/t28a-gui/mobile-navigation.json`. Assign drawer dismissal and focus isolation to GUI owner.

## Browser suite

Full existing Playwright suite: **150 passed, 9 skipped, 0 failed, 0 flaky** across 159 cases (30.7 seconds). A static source classifier found explicit request interception in 125 passing cases; 25 passing cases made no Playwright route interception; 9 unmocked opt-in cases skipped. Classifier method and counts are in `.local/t28a-gui/classify-suite.cjs`; raw run is `.local/t28a-gui/full-suite.json`. Skips: local campaign/controls browser suites, private Gitea runner/finding workflows, non-fixture OIDC installation, live repair, and visual capture baselines. The nine skips are not certification passes.

Focused suite details: `shell.spec.ts` 2 passed unmocked; `connections.spec.ts` 2 passed unmocked; `repair-preview.spec.ts` blocked-state case 1 passed with mocked API. Full suite includes these focused cases.

## Limits and artifacts

This pass does not establish full visual sign-off, every theme/route combination, provider certification, production authentication or G5. Raw browser results and screenshots are in ignored `.local/t28a-gui/`: `live-browser.json`, `operator-path.json`, `blocked-state.json`, `write-reload-cleanup.json`, `mobile-navigation.json`, `full-suite.json`, focused JSON reports and desktop/narrow PNGs.
