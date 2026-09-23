# Organisation OIDC identity configuration

## Contract

Organisation OIDC is optional alongside the installation-wide provider used for initial access and recovery. Only an organisation owner can read or change its OIDC configuration. Configuration uses numeric `If-Match` versions and emits audit events. The client secret is write-only and stored as a vault envelope bound to organisation, config ID and secret version. Responses contain only issuer, client ID, `secret_present`, status, version and verification time. Audit data contains the config ID and version.

The response status is `draft`, `probe_verified`, `active` or `disabled`. A successful metadata probe verifies discovery metadata, not user login; an owner can activate only the current verified version. Organisation login uses `/auth/login?org_id=<UUID>`; login without that parameter remains the installation-provider bootstrap/recovery path. The callback requires an existing membership for the exact verified `(issuer, subject)` pair and never enrolls or links by email.

## Issuer probe boundary

Issuer input must be HTTPS with the default port and no user info, query or fragment. Explicit loopback HTTP is accepted only in development mode. The probe disables proxies and redirects, resolves and pins the selected IP at dial time using the shared `internal/network.IsPublicIP` policy, rejects non-public addresses, uses TLS 1.2+, and bounds time, response body/header bytes and per-host connections. Every request is checked against the exact issuer origin and safe path before dialing, including later token/JWKS requests. Per-organisation probe attempts have a durable 30-second cooldown including failures; each process admits at most four concurrent probes. Cooldown responses return HTTP 429 with `Retry-After`. A failed probe clears prior verification and does not mark the issuer verified. Discovery issuer must match exactly; authorization, token and JWKS endpoints must remain on the issuer origin and within its path. Client secrets are not sent during discovery.

This deliberately excludes private-network issuers and custom HTTPS ports in this slice. Supporting customer-private IdPs or arbitrary hosted tenant issuers needs a separately reviewed outbound network policy; do not loosen the probe to make these routes work.

## Implementation boundary

This slice adds versioned tenant-RLS metadata and separately encrypted secret persistence plus owner-authorized GET/PUT/probe/activate/disable endpoints. Probe status does not imply client credentials were tested. Runtime login state pins organisation, config ID/version, issuer, client ID, state, browser binding, nonce and PKCE verifier. Pre-auth RLS reads only active verified metadata and the one encrypted secret matching that pending configuration. Callback rechecks the exact active version before exchange and session creation, validates issuer, audience, signature, expiry, state, browser binding, nonce and PKCE, and creates a session bound to that organisation/config version. Config rotation, disable or failed verification invalidates existing org sessions on their next request. Sessions last twelve hours while their provider remains active; pending login state lasts ten minutes. RLS stays forced and the runtime role has no bypass.

Existing membership administration still requires a known Account ID. A newly configured IdP therefore authenticates only existing issuer/subject memberships until the separate verified-email-bound, expiring, single-use invitation capability is implemented. Do not auto-link identities or grant membership from email-domain matching.

## Local evidence

`TestOrgOIDCPostgresContract` is an opt-in contract test for an empty disposable PostgreSQL database after migrations are applied. It covers owner-only reads, admin/viewer denial, stale versions, encrypted-at-rest/write-only secret behavior, cross-tenant RLS, local successful and failed metadata probes, durable cooldown after both outcomes, truthful status transitions and activation. `TestOrgOIDCRuntimeLoginCallback` covers the local signed-provider callback and org-bound session contract. Separate focused tests cover concurrent probe capacity, issuer request boundaries and redirects. `REFORGE_TEST_DATABASE_URL` is the runtime role connection string. It is local fixture evidence, not hosted issuer, private-network or live customer certification.
