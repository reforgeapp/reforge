# T29 final browser review

- Revision: `cce6879`.
- Browser target: shared fixture demo at `http://127.0.0.1:8080`.
- Served frontend matches committed build: JS SHA-256 `68e9e0405ab4c8b8834922adf029d207a0db45bc3b3a665de764514683f9595d`; CSS SHA-256 `417cbefa297576f72efcf32498c25ca68ea3b4c4f5a7cde0f7ea6e87e9477c72`. Asset URLs returned HTTP 200.
- Browser: cached Playwright Chromium 1223.
- Command: `PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome npm --prefix web run test:e2e -- --workers=3 --reporter=line`.
- Result: 168 discovered, 154 passed, 14 skipped, 0 failed, 0 flaky; 39.4 seconds.
- Skips are opt-in checks requiring a disposable OIDC issuer, live Gitea/runner, isolated database/controller, or dedicated visual capture mode. No live provider, paid model, or customer integration was used.
- Run log: `.local/t29-final-browser/playwright.log` (ignored). Playwright marker: `web/test-results/.last-run.json`; no failure traces were generated on successful run.
- A first launch attempt used absent `/usr/bin/chromium` and failed before test execution. Cleared only its generated Playwright result files and reran once with cached Chromium above; successful result is the acceptance result.
- This suite verifies local UI/component and fixture-backed journeys against the shared demo. It does not certify live provider behavior, production OIDC, visual review of every route, or G5 external acceptance.
