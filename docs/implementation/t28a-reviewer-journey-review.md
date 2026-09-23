# T28a connected reviewer journey

Date: 2026-09-23

Result: connected negative-path journey passed 1/1 in 50.3s. Server binary built from the working tree at HEAD `af230bd`, including then-current uncommitted source. Identity runtime/login behavior was not exercised or certified.

The opt-in Playwright test ran against fresh isolated PostgreSQL, Reforge at `127.0.0.1:8091`, and disposable Gitea 1.27.3 at `127.0.0.1:53000`. It created a private repository and pull request, enabling Gitea native branch protection requiring one approval throughout. No browser/API interception, mock provider, external/customer repository or native merge was used.

The GUI enrolled a local runner, created two Gitea connections and required the exact new connection's test response to report `healthy`. The test independently opened the sync picker after a page reload and on a separate fresh browser page; both contained the new healthy connection before inventory preview/import. The private runner completed scan/import. A second tenant received 403 for the new connection and imported repository.

The test observed the native change, changed its head and confirmed stale-gate and unqualified merge requests returned exactly 409. Gitea persisted the reviewer request. After reload, the gate showed zero approved reviews. A separate reviewer approved the current head; refreshed gate showed one non-dismissed approval for that exact head. The gate remained `unknown`, native state `blocked`, and merge remained denied because organisation policy, exact-head guard, execution/merge authority, native checks/reviews proof and validation evidence were missing. No native merge occurred.

Validation:

```sh
REFORGE_REVIEWER_ACCEPTANCE_URL=http://127.0.0.1:8091 \
REFORGE_BASE_URL=http://127.0.0.1:8091 \
PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome \
npm --prefix web run test:e2e -- tests/connected-reviewer.spec.ts --workers=1
```

Result: 1 passed (50.3s). The run wrote `.local/t28a-reviewer-journey/steps.log`, `gate-evidence.json`, `native.json`, and blocked/stale 390px screenshots. Containers, named DB volume, copied Gitea state and test server binary were removed; test ports are closed. Test preflight requires a fresh isolated DB with no tenant repositories, connections or runner pools, since previous runs leave Reforge rows after deleting Gitea fixtures.

This verifies only the connected negative path and reviewer/native-provider contract. Positive authorized merge remains open until permitted policy and qualification inputs exist and an approved change is merged through the authorized Reforge path. No G5, provider-version matrix or production isolation certification is claimed.
