# T29aa deep-link review

Run date: 2026-09-24. Source `4e679e52654ebb6643e5486c53c22b31e59fd8a2`, built from a clean archive at `.local/opus-resume/final-gui-4e679e5/source`.

## Build and focused browser check

- Built with `docker build --pull=false --platform linux/amd64 -f deploy/control/Dockerfile -t reforge-t28c-final-control:4e679e5 .`; cached pinned bases reused. Image ID: `sha256:2615ed3ca89efbf8a4639a8724581bd8acd288029d1bd812b79e2d14c12d31ec`.
- Production assets: JS `index-BHuFkyJC.js`, SHA-256 `27643090bbe3886add2d0739e74b88ca01729761b2d8a47481f0e93ae01c37ed`; CSS `index-CS9j70lG.css`, SHA-256 `d5ce48415e7b9f8c465ae3ad6d8ee19cb72e9ad45f8de1a805d2d74869d5d735`.
- Ran `REFORGE_BASE_URL=http://127.0.0.1:18484 PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome npx playwright test tests/changes.spec.ts --workers=1 --reporter=list` from the clean archive’s `web` directory, against the production image with a disposable PostgreSQL 18.6 database. **13 passed, 0 failed**. The API calls are Playwright-intercepted fixtures. The UI selects numeric native change ID `1` after reload and does not select it on a different organization route; this is UI selection coverage, not proof of backend authorization isolation. Normal CSP remained `script-src 'self'`. The Playwright result was returned directly by the test run; the retained `focused-changes-test.txt` records command and summary, not a raw transcript.
- The prior full default suite on source `92e1d03` remains **193 passed, 15 skipped, 0 failed**; it was not rerun for this change.

## Local demo

Updated `.local/opus-resume/demo/start.sh` to use `reforge-t28c-final-control:4e679e5`. The previous `92e1d03` image remains available for rollback (`sha256:0e333dafb93cf7e2bc8f7d44c2713e4ac1f69841aa6da02fda14a2df1a8b066a`). Demo `reforge-t28c-final-demo` is running at `http://127.0.0.1:8084` (container ID `3339c93cd278f345578c19dbba8bd0928bcbf803a348aa8a34312574758971f2`).

The `runtime.env` was left unchanged; the container environment fingerprint matched before and after (`622b7285a7935b5dc9a3761f43ef5918217358fc1fb5948ee3ffe2b82984ac52`). The database `reforge-opus-integration-pg` and docs container `reforge-t28c-current-docs-serve` stayed running with the same IDs. Mounts, read-only root, dropped capabilities, no-new-privileges, and restart policy matched. `/readyz`, fixture-auth metadata/session, served asset hashes, normal CSP, and Help URL `http://127.0.0.1:8082/docs/policies/` (HTTP 200) passed. Audit event `a285465c-ae33-4184-adbf-a5456e5d650a` (`runner.enrollment_created`) is unchanged in `reforge_dev` and returned by the authenticated Audit API after refresh.

The focused test app and temporary database were stopped and removed. Disk headroom is 4,609,736,704 bytes. To restore the requested 3 GiB floor after the image build, only the explicitly authorized regenerable `/tmp/reforge-go-cache` contents were cleared (1,983,050,837 bytes); no Docker cache was pruned. Commands and retained outputs are in `.local/opus-resume/final-gui-4e679e5/logs/`.

These checks use local fixture auth and a local database. They do not certify live providers, human approvals, or G5.
