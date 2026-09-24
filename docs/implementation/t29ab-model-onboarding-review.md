# T29ab model connection onboarding

2026-09-24. Only this ticket resumed; other backlog and broad goal remain paused. Opus5.5 implemented backend and connected acceptance. OpenCode DeepSeek4.1-Flash implemented GUI, regressions and guide. Astra reviewed each slice. Maximum three workers; every completed-worker quota check stayed above20% (latest33%).

## Delivered behavior

Connections → Models & agents → Add connection → Model API → provider → API key → Test connection → model → Save connection. Presets cover OpenAI, Claude/Anthropic, Gemini/Google and OpenCode Zen/Go. Custom endpoints, manual model IDs, CA certificates and approved private runner routing remain available. Subscription/CLI runtime setup remains separate.

Discovery runs through an owner- and CSRF-protected stateless server endpoint. Existing network guard, HTTPS/CA validation, timeout, body/catalog limits and secret redaction apply. Browsing models creates no connection and performs no inference. Saving persists the encrypted credential and probes the selected model.

Provider changes clear stale selections and private settings. Late catalog/save responses cannot populate another organisation or a reopened dialog. Failed post-create probes retry the existing connection with its latest version. A lost create response returns to Connections for reconciliation; it does not offer another create from the uncertain form.

OpenCode public catalogs cannot authenticate a key. Gateway rows/details show amber Catalog only, and inference capabilities remain unknown. Transport health remains the existing backend state; changing it to degraded would prevent otherwise authorized model turns through the existing broker. Unknown gateway model families remain disabled; documented families delegate the native Responses, Messages, Gemini or Chat Completions adapters.

## Local verification

- Reviewed backend commit: `f678a2a`. Scoped Go race checks passed for connections, HTTP API, providers, model adapters and network guard; root independently reran these after the compatible Responses catalog fix. Worker Go vet passed.
- Fresh restricted-role PostgreSQL integration passed73.038s. Owner/cross-org/CSRF/private-address denials precede provider I/O; catalog failures redact upstream secrets and create no persisted connection. Initial reused-database/bootstrap failures and an unrelated agent test flake are retained in worker logs, not counted as passes.
- Catalog contract checks cover all11 supported provider/profile combinations without requiring a saved model. Gateway route tests exercise native-family URLs and unknown-family rejection. Both paid inference and live account certification were excluded.
- API Go/TypeScript generation reproduced byte-identically. Full generator has unrelated pre-existing SQL binding drift; that repair remains paused.
- Full default GUI suite against disposable Go/PostgreSQL backend:209 passed,20 opt-in skipped,0 failed,1.4m. Final focused model suite after gateway badge coverage:16 passed.
- Genuine connected model GUI:4 passed. No API interception: GUI → real server → isolated HTTPS provider fixture → PostgreSQL create/probe → reload. Invalid key redacted; loopback/private/metadata destinations blocked with zero provider traffic. Exactly one connection.created and connection.tested audit record. Secret scans over server logs, artifacts and database dump passed.
- Connected captures cover desktop/390px in light/dark. Root inspected narrow dark catalog and ordinary form. Keyboard/dismissal, stale callbacks and uncertain-save behavior covered by focused browser regressions.

## Evidence and reproduction

- `.local/model-connections/review-go-race.log`, `contract-generation.log`.
- `.local/model-connections/backend.jsonl`: actual backend commands and results, including initial failures and corrected reruns.
- `.local/model-connections/regression/logs/playwright.log`:209/20 default suite.
- `.local/model-connections/polish/test-run.log`:16 focused checks.
- `.local/model-connections/connected/logs/`: backend-connected checks, audit/DB summary, namespace and secret scan.
- `.local/model-connections/connected/artifacts/`: catalog/row light/dark390px/desktop captures.
- `bash scripts/acceptance/model-connections/run.sh` repeats the connected journey. Requires Linux user/network namespaces, PostgreSQL binaries via PGBIN (default /tmp/reforge-postgres/bin), Chromium via PLAYWRIGHT_CHROMIUM_PATH, Go, Node and OpenSSL. Uses disposable storage under .local/model-connections/connected; stops services and removes secret-bearing runtime files on exit. Public-form fixture address exists only on loopback inside its isolated namespace; product network guard is unchanged.

## Remaining certification

External provider credentials and applicable inference budgets are needed to certify live account authentication, entitlement, model availability, tool behavior and billed usage for each advertised provider/profile. No live provider account or paid integration-test API was used. Local fixture acceptance is integration evidence, not vendor certification. Existing subscription runtime qualification and G5 remain open under the paused backlog.

## Delivery

- Backend `f678a2a`; GUI/acceptance `e43569b`; guide `0c3e0b2`. Clean git archives used for container builds. Strict MkDocs build passed inside the pinned docs image.
- Demo: http://127.0.0.1:8084. Guide: http://127.0.0.1:8082/docs/connections/.
- Control image `reforge-model-connections:e43569b`, ID `sha256:623bf2a4682f9138f7d4052f724ff64880eaa0a91581f1c1f75e73ae4ffa4e36`.
- Docs image `reforge-model-connections-docs:0c3e0b2`, ID `sha256:06d8673c1fcf98d1087a8727c22f57295dac02f5fc1331c7a18b03af9cd55815`.
- Served JS `index-CK7D3F81.js`, SHA256 `6d3e9918b902f1dbb2cc173a77f518d7c7724d6320afbbc02e293fedae60178b`; CSS `index-Dz4if7i6.css`, SHA256 `fdfa57e4d3f68a68b5a8aabd68a52c45f1e23c931d7c8c06475a6e39be16a762`. Both byte-matched container assets.
- Actual demo Chromium review passed: all five provider presets, empty-key blocked Test, hidden advanced endpoint, Escape/focus return,390px dark layout and no page errors. No provider request or connection mutation. Captures and verification JSON: `.local/model-connections/demo/`.
- Persistent database, encryption env file (mode0600), artifact mount and audit sentinel `a285465c-ae33-4184-adbf-a5456e5d650a` preserved. Control remains UID1000, read-only, capabilities dropped; docs UID101, read-only. Both local containers restart unless stopped.
- Existing launcher updated: `sh .local/opus-resume/demo/start.sh`. Stop application with `sh .local/opus-resume/demo/stop.sh`. PostgreSQL/docs dependencies remain local. Previous control/docs containers retained stopped with restart disabled for rollback.
- All CLI workers exited. Final quota32% remaining;20% hard-stop threshold was not reached. No other backlog resumed.

