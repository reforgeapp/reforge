# T29ac/T29ad — GitHub App setup and forge onboarding

2026-09-24. Narrow user-authorized work; other backlog/broad goal paused. Stop all jobs at <=20% Codex weekly remaining. Root reviews; max3 Opus/OpenCode workers, exclusive files, no nested workers.

## Frozen interface

- GET `/api/v1/orgs/:orgID/github-app`: `{mode:"manifest"|"hosted"|"unavailable", reason?:string, pending?:{id,phase,app_slug?:string,expires_at,resume_url?:string}}`.
- POST same path + `/setups`, owner+CSRF, body `{name:string,github_org?:string}` →201 `{id,handoff_url}` (same-origin path).
- DELETE same path + `/setups/:id`, owner+CSRF →204.
- Handoff `/auth/github/setup/:id`; manifest callback `/auth/github/manifest/callback`; installation callback `/auth/github/install/callback`; OAuth callback `/auth/github/oauth/callback`.
- All final redirects fixed `/org/{orgID}/connections?github_result=connected|failed|expired|pending_approval&connection={connectionID only on success}`; optional `github_reason` is a bounded public reason code, never raw provider text.
- Self-hosted: official manifest POST → conversion → install → explicit user OAuth with S256 PKCE → ownership verification → persist connection+webhook. Hosted: configured App installation → same explicit OAuth ownership flow. OAuth during installation OFF; explicit OAuth AFTER install callback permits PKCE, state rotation and session binding.
- Installer hint `installation_id` is untrusted. OAuth user must own personal target (account.id=user.id), or be active organisation admin; /user/installations membership plus matching appID required. GET /app/installations confirms app/account and not suspended. Do not accept mere member/repo access as ownership. Verify manifest app owner matches installation target. Invalid/missing install ID: fail actionable restart; existing installation can be reconfigured through official App install page, do not invent unverified candidate select API.
- Fixed github.com/api.github.com guided routes. GHES retains manual App flow; no arbitrary guided hosts.
- Shared hosted App installed once per GitHub account may bind only one Reforge org; global unique claim. Conflict generic, no tenant disclosure. Persist connection/webhook/claim atomically. Recheck Reforge owner and original session at every effect. Hash random state, phases CAS, TTL15min, single use, durable sealed pending secrets/PKCE verifier, wipe after completion/cancel/expiry. Token memory only. If provider failure leaves remote App, give actionable GitHub Apps management link in UI/error help.
- Operator hosted config: REFORGE_GITHUB_APP_ID, _SLUG, _CLIENT_ID, _CLIENT_SECRET_FILE, _PRIVATE_KEY_FILE, _WEBHOOK_SECRET_FILE. All-or-none startup validation; missing hosted config yields actionable unavailable status. Secrets never browser/log/connection API.
- Manifest permissions minimal for actual adapters; organisation Members:read for ownership verification. Derive remaining perms from existing capabilities; no protection bypass/admin write.
- Selfhost webhook: adopt GitHub-returned secret using existing vault binding/path. Hosted: /hooks/github/app HMAC first, installation→tenant binding routing; native lifecycle revoke/suspend must fail closed. Avoid querying arbitrary tenant from unverified payload. Block manual webhook rotation on managed Apps unless implementing corresponding official update.
- PublicURL https required in production. Development loopback allowed with manifest webhook active=false and truthful pending webhook status; docs explain set public HTTPS and update App hook settings. Manual setup unaffected. No hidden automatic tunnels.
- Dedicated handoff page CSP form-action https://github.com only, rest unchanged; no inline script. callbacks/handoff no-store/no-referrer. Fixed final URLs; escaped manifest; bounded timed no-redirect HTTP; no credential query logging.

## GUI and repository import

One coherent provider-first flow. GitHub defaults guided App; token/manual App available. GitLab/Gitea token path retained. Cloud API addresses prefilled/hidden under Advanced; plain names GitHub/GitLab/Gitea. Token labelled Personal access token; manual App RSA PEM upload+multiline, preserve newlines. Instance URL (Gitea/GHES/selfhost GitLab) distinct from optional repository URL. Reject repository URL in API-address field with actionable message; never save invalid github.com/owner/repo endpoint.

After token/manual connection create: test connection, retain failed connection ID for retry (no duplicate create), then actual inventory preview → repository checkboxes → import → Repositories link. GitHub callback success enters same selection step using returned connection ID. Existing forge detail offers Add repositories. Persist server records; URL state/query and scoped cache survive reload/back/tenant switch. Cancellation clears secrets; failed import not success; expiry/revocation handled. Use existing inventory API workflow, not fake client-only repos. Do not infer repository selection from API endpoint. Normal GUI should not require understanding namespace/endpoint/billing-route jargon.

Connection detail forge view: provider/API address/auth/last checked/status + repositories action; remove model/runtime/billing fields. Keep actions grouped, coherent narrow split, no double close buttons. Advanced CA/private runner settings secondary. Keep model/agent/delivery functionality intact. Technical guidance only tooltip/help link; errors actionable and concise.

## Ownership

- backend Opus: internal/** excluding UI, cmd/server/main.go, api/**, generated web/src/api/schema.ts, migration037, backend/integration tests. Existing unrelated dbgen drift must be separately identified; no silent unrelated rollback.
- frontend OpenCode: web/src/app/ConnectionsPage.tsx, new forge/GitHub components, web/src/api/github-app.ts, connections styles; may extract/reuse inventory dialog and update RepositoriesPage only for shared import flow. Router search schema if necessary. Does not edit schema.ts or tests/docs.
- acceptance/docs OpenCode: web/tests/github-app-setup.spec.ts, forge-onboarding.spec.ts, affected existing connection specs only when warranted, deploy/docs/content/connections.md, deploy/compose/.env.example, docs implementation evidence draft. No application source edits.
- root: review, brief/contract/status docs, run verification; fixes delegated.

## Acceptance

Meaningful Go security tests: state/PKCE/replay/expiry/wrong session/role loss, spoofed installation/non-admin, concurrent and cross-tenant claim, vault redaction, restart resume, webhook HMAC/routing, provider failures. Real local restricted-Postgres integration. Browser connected provider fixture proof of token→test→preview→select→import/reload, official-flow handoff/callback, failure retry, keyboard and390px/light/dark; fixture contracts explicitly not live GitHub certification. Unit/mocked browser tests supporting only. Production bundle/containers after local acceptance; preserve demo data. No live external mutations without separate authorization.
