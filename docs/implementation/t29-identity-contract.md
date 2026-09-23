# Organisation OIDC identity configuration

## Contract

Organisation OIDC is optional alongside the installation-wide provider used for initial access and recovery. Only an organisation owner can read or change its OIDC configuration. Configuration uses numeric `If-Match` versions and emits audit events. The client secret is write-only and stored as a vault envelope bound to organisation, config ID and secret version. Responses contain only issuer, client ID, `secret_present`, status, version and verification time. Audit data contains the config ID and version.

The response status is `draft`, `probe_verified` or `disabled`; persistence never claims the config is active. A successful metadata probe verifies discovery metadata, not user login. Activation returns an actionable conflict until org-aware `/auth/login` and callback support exists. No account is enrolled or linked from an email claim alone; future login identity remains the verified `(issuer, subject)` pair.

## Issuer probe boundary

Issuer input must be HTTPS with the default port and no user info, query or fragment. Explicit loopback HTTP is accepted only in development mode. The probe disables proxies and redirects, resolves and pins the selected IP at dial time using the shared `internal/network.IsPublicIP` policy, rejects non-public addresses, uses TLS 1.2+, and bounds time, response body/header bytes and per-host connections. Every request is checked against the exact issuer origin and safe path before dialing, including later token/JWKS requests. Per-organisation probe attempts have a durable 30-second cooldown including failures; each process admits at most four concurrent probes. Cooldown responses return HTTP 429 with `Retry-After`. A failed probe clears prior verification and does not mark the issuer verified. Discovery issuer must match exactly; authorization, token and JWKS endpoints must remain on the issuer origin and within its path. Client secrets are not sent during discovery.

This deliberately excludes private-network issuers and custom HTTPS ports in this slice. Supporting customer-private IdPs or arbitrary hosted tenant issuers needs a separately reviewed outbound network policy; do not loosen the probe to make these routes work.

## Implementation boundary

This slice adds versioned tenant-RLS metadata and separately encrypted secret persistence plus owner-authorized GET/PUT/probe/activate/disable endpoints. Probe status does not imply client credentials were tested. Activation remains unavailable because the runtime still builds a single OIDC verifier from installation configuration and login state has no organisation binding. The future login entry is `/auth/login?org_id=<UUID>`; requests without an organisation continue to use the installation IdP as the bootstrap/recovery fallback. The pre-auth lookup must use a transaction-local login-org context and a narrow metadata-only RLS policy; never disable RLS or grant a global bypass. Keep encrypted envelopes in the separate `org_oidc_secrets` table, with a distinct pending-login policy pinned to org, config ID and version if callback exchange needs the secret.

Follow-on work must pin organisation, config ID/version and issuer into durable login state; load the exact version through the hardened client; validate issuer, audience, signature, expiry, state, browser binding, nonce and PKCE; and create a session under the documented org policy. Define session lifetime for users with multiple organisations before implementing per-org expiry. Existing membership administration continues to require the Account ID of a user who has already logged in. Add verified-email-bound, expiring, single-use invitations as a separate capability; do not auto-link identities or grant membership from email-domain matching.

## Local evidence

`TestOrgOIDCPostgresContract` is an opt-in contract test for an empty disposable PostgreSQL database after migrations are applied. It covers owner-only reads, admin/viewer denial, stale versions, encrypted-at-rest/write-only secret behavior, cross-tenant RLS, local successful and failed metadata probes, durable cooldown after both outcomes, truthful verification state and the activation block. Separate focused tests cover concurrent probe capacity, issuer request boundaries and redirects. `REFORGE_TEST_DATABASE_URL` is the runtime role connection string. It is local fixture evidence, not hosted issuer, private-network or live customer certification.
