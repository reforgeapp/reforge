# T02 identity and tenancy

Status: locally verified against PostgreSQL 18.6 and a signed local OIDC provider. External OIDC certification remains pending.

`auth.New(ctx, store, auth.Config)` validates the public origin and explicit loopback development configuration. Production self-hosting requires an OIDC issuer/client and redirect URI `{public_url}/auth/callback`. OIDC uses durable ten-minute state, browser binding, nonce, S256 PKCE, verified issuer/audience/signature/expiry, and a bounded HTTP client. Sessions last twelve hours; opaque cookie values are stored only as SHA-256 hashes. HTTPS cookies use `__Host-`, Secure, HttpOnly, Path=/ and SameSite=Lax. Writes require exact Origin and session CSRF token; all identity routes enforce the configured Host.

The runtime uses FORCE RLS for every table. Identity lookups bind the verified issuer/subject or unpredictable session/state/bootstrap hash; organisation reads/writes use tenant transactions. Composite foreign keys constrain team and repository bindings. Membership DTOs contain direct repository grants; resolved actors include the current union of direct and team grants. Roles do not imply repository access or provider authority.

## Routes

- `GET /auth/login`, `GET /auth/callback`: OIDC redirects; explicit development fixture login persists a development owner and organisation.
- `GET /api/v1/session`: user, organisations, memberships and `csrf_token`; unauthenticated requests return 401.
- `POST /auth/logout`: revoke current session and clear its cookie.
- `DELETE /api/v1/session`: revoke all sessions belonging to the authenticated user.
- `POST /auth/bootstrap`: authenticated OIDC session, Origin/CSRF and JSON `{token,name}`; returns the new organisation with status 201. Configure a token of at least 32 characters and an absolute expiry. The persisted singleton is consumed atomically and cannot be reset by restarting the application. Hosted edition rejects bootstrap.
- `GET /api/v1/orgs/{orgID}/memberships`: owner-only paginated membership list.
- `PUT|DELETE /api/v1/orgs/{orgID}/memberships/{userID}`: owner-only membership administration; PUT accepts role, all_repositories, team_ids and direct repository_ids. New members must already have an OIDC identity; they can share the user ID returned by their session endpoint.
- `GET /api/v1/orgs/{orgID}/teams`: paginated teams restricted to the current actor's scope.
- `PUT|DELETE /api/v1/orgs/{orgID}/teams/{teamID}`: owner/admin team administration. PUT accepts name and repository_ids. Scoped admins can edit their assigned teams using repositories they can currently access; creating/deleting teams requires organisation-wide repository access.

Lists accept `limit` (1–200, default 50) and UUID `cursor`. Configuration PUT/DELETE requires quoted numeric `If-Match`; use `"0"` for creation. Edits advance version and produce an audit event in the same transaction. Membership changes serialize on the organisation row and protect the last owner, including concurrent removals. Mutations lock organisation, session, then load current authority; queued requests cannot retain revoked membership. Mutation retries with stale versions return conflict.

## Service integration

`Server.IdentitySession()` checks cookie, Host, Origin and CSRF, then stores the authenticated session for `httpapi.SessionFromContext`. `WithActor` and `ResolveActor` recheck the live session and current membership. `WithMutation` additionally serializes authorisation with membership and session revocation. `RequireRepository` verifies a repository directly; `ResolveRepository` accepts a trusted server-side child lookup running inside the tenant transaction. Use the resulting actor and explicit repository predicates for collection queries. Repeat these helpers for event batches and artifact authorization so revocation takes effect without waiting for session expiry.

## Evidence and remaining integration

Executed with `-race`: production cookie/Origin checks; unauthenticated and hostile-Host requests; CSRF rejection; cross-tenant/team/repository and child resolution; unscoped RLS denial; version conflicts; concurrent last-owner removal; queued mutation versus membership revocation; membership roundtrip followed by team revocation; logout and saved-session revocation; signed local OIDC PKCE/browser binding/state replay/nonce validation, including callback after service recreation; bootstrap consumption and expiry.

Actual SSE termination and artifact download routes belong to later tickets and must use the same resolver; they are not claimed as implemented here. Expired session/login row retention and rate limiting remain operational follow-ups. The repository connection column is temporarily nullable; T03 adds its tenant-composite connection foreign key. Coordinator owns migration002, startup wiring, OpenAPI and dependency manifests.
