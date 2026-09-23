# T28a opt-in browser review

Run date: 2026-09-24. Five corrected repository opt-in specs run against current production frontend bundle and isolated fixture-auth PostgreSQL. Local evidence only; no provider certification or G5 claim. This is not a fresh run of the 193-test production-browser suite.

## Build and isolation

- Source base: `675b0fd`; current repository HEAD: `4c03722b15baf094852769a51b9adf4cb3d96f53`.
- Product diff from `675b0fd` (`web/src/styles/app.css`, `web/src/app/FindingsPage.tsx`) SHA-256: `cc774279da409ee6ac162d1b233edfae4be2d03491ff31dc0d71e3214ad35b4c`.
- Production frontend build: `npm --prefix web run build -- --outDir /home/mnorris/repos/reforge/.local/opus-resume/optin-browser/build/web-dist --emptyOutDir`; edited spec typecheck: `(cd web && npx tsc --noEmit --strict --module esnext --moduleResolution bundler --target es2022 --types node --skipLibCheck tests/{connected-acceptance,connections-mobile,findings,insights-live,campaigns-live}.spec.ts)`; TypeScript and Vite build passed. Bundle: CSS `index-CS9j70lG.css`, SHA-256 `d5ce48415e7b9f8c465ae3ad6d8ee19cb72e9ad45f8de1a805d2d74869d5d735`; JS `index-U6jaT4rW.js`, SHA-256 `daf963bb65f4e2d92ea933c6226c6a54131909fe3a5eed40cdc732f059f717b2`.
- Server binary SHA-256: `fe052c4455eb8f53133aba18219fb3e57650de6b02523a0c1104d3bd5508a478`. Served CSS/JS hashes matched build output in `run-current-20260924/logs/served-qa.txt` and `served-qc.txt`.
- Command: `OUT=run-current-20260924 WEBDIR=/home/mnorris/repos/reforge/.local/opus-resume/optin-browser/build/web-dist SPECROOT=/home/mnorris/repos/reforge/web SERVER=/home/mnorris/repos/reforge/.local/opus-resume/production-browser/reforge-server-final .local/opus-resume/optin-browser/run.sh qa qc cap qn`.
- Runner used user+network namespaces with loopback only. PostgreSQL bound inside namespace on `127.0.0.1:55432`; app bound inside namespace on `8090`/`8092`. `reforge_runtime` was NOSUPERUSER/NOBYPASSRLS; `reforge_migrator` had migration-only BYPASSRLS. Separate namespace-local `reforge_optin` and `reforge_dev` databases. No demo/docs/host Postgres mutation. Namespace services stopped on exit.

## Results

All six live GUI scenarios passed across five repository specs: connected acceptance (2), connections mobile (1), findings live (1), insights live (1), campaigns live (1). Existing mobile-navigation/theme/findings smoke specs: 12 passed, 1 skipped. Review capture spec: 2 passed.

- `connected-acceptance.spec.ts`: team CRUD and org scope passed; artifacts directory is created by test itself.
- `connections-mobile.spec.ts`: mobile/desktop containment, persistence, keyboard detail and close behavior passed. Card layout no longer requires text wrapping.
- `findings.spec.ts`: assignment began with empty action reason; Unassign was enabled; empty-reason unassignment persisted; Dismiss remained disabled until reason supplied; subsequent dismiss/reopen/snooze persisted.
- `insights-live.spec.ts`: real local policy, simulation/activation, budget, and audit export passed.
- `campaigns-live.spec.ts`: wizard asserted Members → Execution → Rollout; lifecycle persisted; reload restored selected campaign detail and URL; budget scope remained campaign-specific.

Review captures: `.local/opus-resume/optin-browser/run-current-20260924/artifacts/captures/` contains closed/open 390px sidebar screenshots for light and dark themes, measured shadow state, and findings empty-reason screenshots at 390px/1440px. Closed sidebar computed shadow was `none`; open sidebar had shadow in both themes.

Original unmodified failures and diagnostic results are preserved in [baseline review](t28a-optin-baseline-review.md). The first narrow Findings capture caught a sidebar transition; settled recapture is pending and it is not accepted visual evidence.

## Scope and limits

Product changes limited to moving mobile sidebar shadow onto `.sidebar.sidebar-open` and removing the unused reason requirement from Unassign. Busy and CSRF guards remain; snooze/dismiss/reopen still require reasons. Spec edits update UI selectors, explicit campaign step assertions, and evidence output paths; no scope, version, tenant, or authorization checks were weakened. Product diff is available for coordinator review.

Fixture auth and SQL-seeded repositories/orgs do not certify production identity or provider imports. Gitea records used `.invalid` endpoints; campaigns had no dispatch authority. Do not infer the unrun 193-test suite from these opt-ins.
