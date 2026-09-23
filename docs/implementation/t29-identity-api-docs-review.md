# T29 identity API docs review

## Contract added

OpenAPI now covers organisation OIDC settings (`GET`/`PUT`), metadata probe, activation, disable, invitation list/create/revoke, and public invitation redemption. Generated Go models and browser TypeScript schema match these endpoints and response shapes.

Settings and invitation-management reads require an authenticated owner session. Mutations require exact configured Origin and session CSRF token; OIDC mutations also require quoted numeric `If-Match`; missing header returns `428 precondition_required`, malformed version returns `400 invalid_request`, and stale version returns `409 conflict`. Invitation paging defaults to 50 and accepts `limit=1..200` plus UUID cursor.

Settings response exposes `configured`, `secret_present`, optional `issuer` and `client_id`, `status`, `version`, optional `verified_at`, `verified`, `activation_available`, and optional `activation_blocked`. Secret value is never returned. Status values are `unconfigured`, `draft`, `probe_verified`, `active`, and `disabled`. Saving a changed configuration creates a new draft version and clears verification. Probe verifies discovery metadata only; it does not validate credentials or complete login. Probe failure returns `422 issuer_unverified`; durable cooldown and process capacity return `429` with `Retry-After`. Activation requires the current verified version and returns `204`; disable increments version and clears verification.

Invitation creation is owner-only and requires an active verified provider. Listing and revocation are owner-only; they remain available if provider is disabled. Create request contains normalized email, any supported role (`owner`, `admin`, `maintainer`, `reviewer`, `viewer`), and expiry from one hour to 30 days. The `201` response contains invitation fields and the one-time `redemption_url`; later list responses never include token material. List returns `items`, optional UUID `next_cursor`, and `complete`. Revoke returns `204` for an unused invitation.

Public `POST /auth/invitations/redeem` requires exact same-origin `Origin`, `application/x-www-form-urlencoded`, one 43-character `token` field, no query string, and request body no larger than 8 KiB. No session is required. It returns `200 {"authorization_url":"..."}`, sets `Referrer-Policy: no-referrer` and `Cache-Control: no-store`, and rejects tokens placed in request URLs. Browser flow removes `/invite#token=...` before posting the token in the form body.

## Operator guide

Admin guide now describes owner navigation and OIDC save/probe/activate/disable flow, separates operator-wide identity from organisation identity, and documents invitation copy/accept/revoke behavior and supported roles/expiries. It states that local signed-provider contract coverage does not certify hosted customer IdPs. Existing provider network limits remain documented: HTTPS/default port, public-address-only probes; private-network issuers and custom HTTPS ports are unsupported.

## Evidence boundary

The local signed identity-provider fixture exercises successful and failed metadata probes, activation, invitations, and callback behavior under the restricted runtime role. It is local contract evidence only. No hosted customer identity provider has been certified; a successful metadata probe does not establish hosted login compatibility.

## Checks

Worker checks passed: OpenAPI JSON parse, Go and TypeScript generation idempotence, browser typecheck, representative invitation-creation response validation against the JSON schema, and `git diff --check`. Coordinator semantic comparison found seven added paths and seven added schemas, with no changes to existing paths or schemas. Strict MkDocs image build follows with the frozen source revision.
