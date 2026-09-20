# Local functional demo

This runbook describes the disposable self-hosted demo. It does not certify external GitHub/GitLab services, production subscription custody, or a clean-install deployment.

## Prerequisites

- Local PostgreSQL 18.6 is available on `127.0.0.1:55432`.
- Development databases are `reforge_dev` and `reforge_test`.
- Local credentials remain in ignored, mode-0600 `.local/development.env`; do not print or commit them.
- Cached Chromium is available at `/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome` when browser checks need an explicit executable.

Check PostgreSQL readiness without changing services:

```sh
/tmp/reforge-postgres/bin/pg_ctl -D .local/postgres status
```

Load local variables only in the shell that needs them:

```sh
set -a
source .local/development.env
set +a
```

Apply migrations with the migration role and environment described in [local development](local-development.md). Do not edit migrations that are already applied.

Build and start the current self-hosted service from the repository root:

```sh
export REFORGE_REPAIR_IMAGES='{"go":"sha256:e3c05191a7d519185395855f95ef9e9a51f5359152bc59f644215a28ba962852"}'
make dev
```

`make dev` runs the server in the foreground at `http://127.0.0.1:8080`. Stop it with `Ctrl-C`; rebuild after source changes and run `make dev` again to restart.

The retained repair demo uses the separate local service at `http://127.0.0.1:8081`. Its current blocked run is [b1cca48f-eab5-4ee3-adac-0a7873743926](http://127.0.0.1:8081/org/00000000-0000-4000-8000-000000000001/runs?run=b1cca48f-eab5-4ee3-adac-0a7873743926). Stop and clean that demo with:

```sh
python3 scripts/test-repair-browser.py --cleanup-demo
```

## Browser entrypoint

The local browser uses `http://127.0.0.1:8080`. Sign in through `/auth/login`, then choose an organisation. The functional areas are campaigns, usage, deployments, GitOps promotions, audit, findings, and runs.

Run the campaign browser checks from `web/` with the pinned browser when the local server is ready:

```sh
PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome npx playwright test tests/campaigns.spec.ts
```

The live campaign check is explicit and requires the disposable development database:

```sh
REFORGE_LIVE_CAMPAIGNS_BROWSER=1 PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome npx playwright test tests/campaigns-live.spec.ts
```

The browser entrypoint is `http://127.0.0.1:8080/auth/login`; after fixture login, open `/org/{orgID}/campaigns`. The live check must leave no external provider writes.

## Evidence

- T24 controls: `.local/insights-browser-final.log`, `.local/insights-pg-root3.log`.
- T23 GitOps/native acceptance: `.local/gitops-final-pg-native2.log`, `.local/gitops-browser-root2.log`.
- T25 targeted backend race: `.local/frozen-targeted-race-final.log` (17.740s), including pre-dispatch cancellation and startup callback ordering.
- T25 campaign reports awaiting final review: `.local/campaign-fairness1.log`, `.local/campaign-repair2.log`, `.local/campaign-browser2.log`. `.local/campaign-browser-live1.log` is a failed attempt superseded by `campaign-browser2.log`.

Evidence from fixture providers or disposable Gitea supports local behavior only. It is not external certification.

## Remaining work

T25 needs fuller controller/native acceptance and final review. Pre-dispatch cancellation is covered by the targeted race; durable campaign/browser acceptance remains pending. T16 production custody/runtime wiring, T17 qualification flow, and T26–T28 remain deferred. G5 is open.
