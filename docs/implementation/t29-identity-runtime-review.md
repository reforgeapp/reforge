# Organisation OIDC runtime review

## Runtime contract

`GET /auth/login?org_id=<UUID>` starts organisation OIDC only when that organisation has an active, current, probe-verified provider. `GET /auth/login` without `org_id` keeps using the installation provider for bootstrap and recovery. Callback state is one-use, expires after ten minutes, and binds the organisation, configuration ID/version, issuer, client ID, nonce, PKCE verifier and browser cookie hash.

Callback rechecks the exact active configuration before token exchange and again before creating a session. Secret lookup uses the organisation/config/version-bound vault envelope. The callback validates state, browser binding, PKCE, issuer, audience, signature, expiry and nonce. It looks up the exact `(issuer, subject)` account and requires existing membership in the requested organisation. It does not create an account, infer identity from email, or grant membership.

New sessions persist organisation, OIDC config ID and config version. They expose only that organisation and cannot switch to another membership. Authentication and actor resolution require the same provider version to remain active and verified. Configuration edit, disable, failed probe, or version change invalidates those sessions on their next request without broad session-table policy or bulk revocation. Session lifetime remains twelve hours while provider stays active; pending login state lasts ten minutes.

## Isolation and outbound boundary

Pre-authentication reads use transaction-local login context and RLS policies restricted to one active/verified config, then one secret row matching its organisation, config ID and version. Existing tenant RLS remains forced; runtime role has no bypass. Pending callback state is deleted before exchange, so failed exchanges require a fresh login.

Discovery, authorization, token and JWKS endpoints come from one validated discovery response and must stay on the exact issuer origin and safe path. Requests use the hardened issuer transport: no proxy or redirects, public-IP/DNS pin checks, development-only loopback HTTP, TLS 1.2+, bounded header/body/time, and bounded per-process concurrency. Client secret is sent only to the token endpoint.

## Enrollment limit

An organisation IdP can authenticate only identities that already have membership under that exact issuer and subject. Current membership management requires an existing account ID, so first-time customer onboarding through a newly configured IdP needs the separately scoped verified-email-bound invitation flow. Do not enable email linking or automatic enrollment as a workaround.

## Local evidence

The restricted-role PostgreSQL runtime test uses a local signed OIDC provider and verifies callback completion after service restart; state expiry and replay rejection; wrong nonce, audience and issuer rejection without session issuance; required pre-existing membership; no account creation on unknown subject; cross-organisation session denial; and pending-callback plus session invalidation after config rotation and provider disable. Focused auth/API race tests, package vet and diff checks are run before review. Local signed-provider evidence does not certify hosted identity-provider interoperability or customer onboarding.
