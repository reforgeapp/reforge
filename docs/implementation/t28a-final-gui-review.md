# T28a final GUI review

Run date: 2026-09-24. Integrated source `92e1d031dd18e4202128a91fa3805b2c49feacb7`; immutable clean archive: `.local/opus-resume/final-gui-92e1d03/source`. No product source changes followed this commit.

## Build and browser suite

- Built control image from clean archive with `docker build --pull=false --platform linux/amd64 -f deploy/control/Dockerfile -t reforge-t28c-final-control:92e1d03 .`; pinned base images and Docker cache were reused, no base updates or cache prune. Image ID `sha256:0e333dafb93cf7e2bc8f7d44c2713e4ac1f69841aa6da02fda14a2df1a8b066a`.
- Extracted image assets exactly match accepted opt-in bundle: JS `index-U6jaT4rW.js` SHA-256 `daf963bb65f4e2d92ea933c6226c6a54131909fe3a5eed40cdc732f059f717b2`; CSS `index-CS9j70lG.css` SHA-256 `d5ce48415e7b9f8c465ae3ad6d8ee19cb72e9ad45f8de1a805d2d74869d5d735`.
- Full default Playwright suite ran once from clean archive against this production image and disposable PostgreSQL 18.6. Command: `REFORGE_BASE_URL=http://127.0.0.1:18484 PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome npx playwright test --workers=1 --reporter=list --output /home/mnorris/repos/reforge/.local/opus-resume/final-gui-92e1d03/results`.
- Result: **193 passed, 15 skipped, 0 failed** (208 total, 1.9 minutes). Playwright summary is passed with no failed tests. Raw log and command are `.local/opus-resume/final-gui-92e1d03/logs/full-suite.log` and `full-suite-command.txt`.
- The skips require opt-in or external services: 5 connected acceptance/mobile cases, 3 visual capture cases, live campaigns, connected reviewer, live Gitea connection lifecycle, live findings, live insights, live repair, and non-development OIDC install. Skip list is in the raw log; none failed.
- Test app listened on loopback port `18484`; disposable Postgres used tmpfs data and a private Docker network, with only its own database published on loopback `55440`. Runtime role was NOSUPERUSER/NOBYPASSRLS; migrator was NOSUPERUSER/BYPASSRLS. Both test services and the private network were stopped/removed after checks.

## Findings capture

The first 390px capture showed the drawer mid-transition and is preserved as `.local/opus-resume/final-gui-92e1d03/artifacts/findings-unassign-empty-reason-390-rejected.png`. Settled recapture passed after checking sidebar right edge ≤1px, no Close navigation button/backdrop, `.main-column` inert=false, and the full Unassign button in viewport. Accepted screenshot: `.local/opus-resume/final-gui-92e1d03/artifacts/findings-unassign-empty-reason-390-settled.png`. Capture test log: `.local/opus-resume/final-gui-92e1d03/logs/capture.log`. Root visually reviewed and accepted the settled image.

## Local demo

Updated `.local/opus-resume/demo/start.sh` image tag to `reforge-t28c-final-control:92e1d03`. The old rollback image remains available as `reforge-t28c-final-control:675b0fd` (image ID `sha256:59819fbd3a751b824117728157934a1b75af1228901848d1c9794c5499f17da0`). Demo container is running at `http://127.0.0.1:8084` as `reforge-t28c-final-demo` (ID `c5da15c5ac3ba03f2c373021979c62d3046705fa0f9a6c24031f6cb6737e51c8`).

The existing `runtime.env` is unchanged (SHA-256 `d1e85c7df914b681f5708d72e229599dd46b8434b1b7a2d19ad21c9c7035ee09`); container env fingerprint matched before/after. PostgreSQL `reforge-opus-integration-pg` (ID `01254123d2621af9a1424d9ab5cef90196e42c007fc5f945da25b796902f11d4`) and Docs container `reforge-t28c-current-docs-serve` (ID `a3f84f11b26ee2167446ada2b34764847cf48f0f61b93c869830c499e5a8b526`) stayed running. The existing artifact mount and hardened container settings remain in place.

After refresh, `/readyz`, fixture-auth metadata, authenticated session, served JS/CSS hashes, normal CSP, and Help URL `http://127.0.0.1:8082/docs/policies/` (HTTP 200) passed. Audit event `a285465c-ae33-4184-adbf-a5456e5d650a` (`runner.enrollment_created`) remained in the same `reforge_dev` database and was returned through the authenticated Audit API after restart. Details: `.local/opus-resume/final-gui-92e1d03/logs/demo-verification.json`; pre-refresh record is `demo-audit-before.txt`.

These checks use local fixture auth and local database only. They do not certify live providers, human approvals, or G5.
