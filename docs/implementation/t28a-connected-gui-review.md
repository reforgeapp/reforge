# T28a connected GUI acceptance review — 2026-09-23

Scope: current Overview, Organisation and Connections routes against an isolated Go/PostgreSQL instance. This supplements, but does not replace, the broader T29 GUI review or J01–J10 acceptance.

## Run and build

- Source revision: `040ca09` plus this uncommitted test/report. Frontend TypeScript check and production Vite build passed into `.local/t28a-connected-gui/web-dist`; Go server built from the same worktree.
- Dedicated PostgreSQL 18.6 container on loopback port 55434; Go server on `127.0.0.1:8090`. Runtime role had neither superuser nor `BYPASSRLS` privileges. Existing demo at port 8080 and its database were not used for writes.
- GUI assets served by the isolated server matched local build: `index-Crtub4So.js` SHA-256 `a9f32933dce579f0ec96f87748b021deaccf4de7ac2804c03401053bf83e5adb`; `index-Cp7V3BTd.css` SHA-256 `c697db88524244cf6e29fba8ffaaa7dd5fb89513a6efd32699de271723c45328`.
- Focused Chromium Playwright run: 2 passed, 0 failed. The mutating spec skips unless `REFORGE_BASE_URL` and `REFORGE_CONNECTED_ACCEPTANCE_URL` both explicitly select `http://127.0.0.1:8090`, so the default shared demo URL cannot receive writes. No Playwright route/session/API interceptions. Fixture authentication was explicitly enabled. Development organisation came from fixture login; second organisation and membership were seeded directly in this disposable database. The GUI performed all tested reads and writes.

## Observed journeys

| Check | Result | Evidence type |
| --- | --- | --- |
| Fixture login and session load | Pass; session listed two persisted organisations and GUI showed fixture-auth banner | Local auth fixture + real backend/database |
| Organisation switch | Pass; switching preserved route and loaded each organisation’s real data | Real backend/database |
| Team create/readback | Pass; GUI `PUT /teams/{id}` returned 200, team remained after repository navigation and page reload; same team absent from Development org | Real GUI mutation and PostgreSQL persistence |
| Unsupported Codex runtime | Pass; GUI created a Codex configuration pointed at reserved `.invalid` endpoint; qualification read returned 200; official sign-in stayed disabled with missing isolated-runtime/credential-custody remediation | Local test configuration; no external runtime call |
| Keyboard navigation | Pass; Enter opened mobile drawer, focus moved into it, background became inert, Escape restored focus; route selection closed drawer | Real backend session, browser keyboard |
| Connection dialog | Pass; focused dialog stayed in viewport at 390px, Tab remained in dialog, Escape closed it | Real backend session, browser keyboard |
| Responsive and theme captures | Pass for current Overview at 390×844 and 1440×900, light and dark. Captures were visually inspected after waiting for the mobile drawer transition to settle | Current build only |

Structured server request logs record the team `PUT` 200, team list `GET` 200, Codex agent `POST` 201 and qualification `GET` 200. Browser request paths are in `.local/t28a-connected-gui/artifacts/backend-requests.json`; server logs are `.local/t28a-connected-gui/server.log`.

Screenshots: `.local/t28a-connected-gui/artifacts/overview-{390,1440}-{light,dark}.png`.

## Acceptance boundary

This slice gives current real-backend evidence for a narrow part of V20/J10 and V27: ordinary GUI reads/writes, organisation-specific readback, keyboard use and an actionable disabled subscription-runtime state. Fixture authentication and the manually seeded second organisation are not production identity evidence. The `.invalid` Codex record proves only disabled-state behavior, not subscription eligibility or provider integration.

It does not finish J01–J10 or V20. It covers three routes, one ordinary team CRUD mutation and one disabled-route configuration; logs/artifact sanitization, provider approval flows, repair/publication, deployment health, complete error-state coverage and hosted tenant corpus remain with their owning tickets. The four captures have no before image with identical data/state, so they do not alone satisfy V29 comparative review. Existing GUI matrix evidence remains separate. V23 all-route review and G5 remain open.

## Reproduction

Build current GUI into the isolated run directory with `npm --prefix web run build -- --outDir ../.local/t28a-connected-gui/web-dist`; build server with `go build -trimpath -o .local/t28a-connected-gui/reforge ./cmd/server`. Start a fresh local PostgreSQL 18.6 instance with migrations and runtime grants, then launch the server with fixture auth and web root `.local/t28a-connected-gui/web-dist`. Run the spec only with both URL variables set to `http://127.0.0.1:8090`, plus `PLAYWRIGHT_CHROMIUM_PATH`. Temporary credentials and database were removed during teardown; preserved build, request-log and screenshot artifacts contain no credentials. Normal development setup remains documented in `docs/implementation/local-development.md`.
