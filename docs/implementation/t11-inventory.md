# T11 inventory and event orchestration

Local implementation complete; coordinator owns startup, provider read gateway, OpenAPI and migrations 010–012. No forge mutations are performed by this service.

## Service boundary

`inventory.New(db, identity, vault, reader, decoder)` accepts the fixed-operation `providers.Service.Read` gateway and `providers.DecodeWebhook`. `Run(ctx, workerID)` runs the durable scheduler; `Claim` and `Step` are trusted controller methods, without browser or runner routes.

Each provider read revalidates the database job fence, lease, current owner membership where applicable, namespace and connection version inside the gateway transaction. Results persist in a separate organisation/job transaction that repeats those checks. A credential rotation or expired/replaced lease prevents results from committing. Thirty-second leases cover one bounded read or import page; recovery resumes the last committed cursor. Reads have a twelve-second outer deadline; the gateway bounds execution to ten seconds.

Tenant iteration persists a singleton UUID cursor before processing each organisation, releasing its global lock before tenant work. The catalogue and cursor expose only UUIDs; catalogue writes remain tenant constrained, cursor writes require the trusted scheduler transaction setting. Connection `last_served` and job `last_claimed` provide durable fair ordering. Each pass examines at most fifty idle tenants.

## API

All organisation routes use the current session, Host, origin and CSRF checks. Inventory administration requires an owner with all-repository access. Repository and change reads filter by current effective direct/team grants.

| Route under `/api/v1/orgs/:orgID` | Operation |
| --- | --- |
| `POST /inventory-syncs` | Queue a preview scan with `connection_id` and optional namespace matching connection settings; returns 202 |
| `GET /inventory-syncs`, `GET /inventory-syncs/:syncID` | Paged jobs and status |
| `DELETE /inventory-syncs/:syncID` | Cancel remaining work with `If-Match`; invalidate active fence |
| `GET /inventory-syncs/:syncID/candidates` | Paged preview, deterministic native-ID cursor |
| `POST /inventory-syncs/:syncID/import` | Queue `all:true` or selected `native_ids`, optional `team_ids`; completed scan `If-Match` required |
| `GET /repositories`, `GET /repositories/:repositoryID` | Scoped records and freshness |
| `GET /repositories/:repositoryID/changes` | Scoped canonical change cache and snapshot freshness |
| `GET`, `PUT`, `DELETE /connections/:connectionID/webhook` | Inspect, create/rotate or revoke webhook endpoint |

Repository filters are `q` (literal case-insensitive substring), `provider`, `team_id`, and `status` (`active`, `archived`, `paused`, `missing`, `stale`), applied before keyset pagination. Pages contain at most 200 records. Change pages distinguish pagination `complete` from `snapshot_state`, `observed_at` and `connection_version`.

Webhook creation uses `If-Match: "0"`; subsequent rotation/revocation requires the returned version. Only create/rotate returns the new secret, with `Cache-Control: no-store`. GET and audit/events contain no secret. Install the returned relative path beneath the configured public origin in the forge. Forge webhook registration remains an explicit administrator operation.

## Sync and import semantics

Full scans use only the namespace configured on the connection. Candidates are staged; scans never implicitly import repositories. Imports accept the latest completed scan no older than one hour, validate selected IDs, then persist at most 100 repositories per transaction. Team versions and the initiating owner's current membership are revalidated for every batch. Cancellation or a changed grant stops remaining batches; already imported records remain.

Repository identity is `(organisation, connection, native ID)`, so renames retain application IDs, team grants and history. Only a completed scan can mark an imported repository absent from the same namespace inaccessible. Partial, repeated, non-advancing or inconsistent pages mark observations stale and preserve last-known access. There is no hard deletion from inventory absence. Archived repositories remain visible; `RequireFreshTx` rejects them for effect eligibility.

A scan supports up to 1,000 pages / 100,000 observations. Import requests select at most 10,000 native IDs, or all staged candidates. Active jobs are capped at 1,000 per organisation. Retention removes unreferenced finished jobs/candidates after thirty days in batches of fifty.

`RequireFreshTx` requires current connection/scope, accessible non-archived repository, completed inventory observation within one hour, and a completed change snapshot within fifteen minutes. Callers must also hold their mutation/lease/policy authorization boundary. It does not grant provider write authority. Partial change pages may update individual cached records, but snapshot freshness is cleared before refresh and remains stale until every page completes. Missing changes become `unknown`, never inferred closed or merged.

## Events and polling

`POST /hooks/v1/:orgID/:endpointID` resolves the enrolled endpoint before verification. Secrets are separately envelope encrypted, bound to organisation, endpoint UUID and version; forge tokens are not reused. Bodies are bounded to 1 MiB. Delivery IDs and payload digests are deduplicated independently, including changed unsigned delivery headers. Dedup retention is seven days, capped at 100,000 rows per organisation.

Verified payloads only identify an already imported native repository. Payload names, SHAs and change state never populate canonical records. Duplicate/out-of-order events coalesce into repository refreshes; an event during an active refresh schedules another read after completion. Unknown repositories accelerate a full preview scan without importing them.

GitHub/Gitea secrets are random opaque HMAC keys. GitLab secrets use `whsec_` plus standard base64 for a 32-byte key, compatible with the documented `signing_token` field and the adapter's timestamped signature verifier; legacy token verification remains available. [GitLab project webhook API](https://docs.gitlab.com/api/project_webhooks/)

Full scans poll every fifteen minutes; imported repository/change refreshes poll every five minutes. Transient errors retry up to eight times, honoring bounded Retry-After with deterministic jitter. Terminal access/capability errors stop affected automatic polling and expose an actionable state; an owner explicitly starts a new sync after correcting access. Job failures retain sanitized reasons. Repository sync/change events and owner-only job completion events use the transactional workflow event log and its current-scope replay checks.

## Verification

Disposable PostgreSQL 18.6, runtime RLS role:

- `go test -race ./test/integration -run '^TestInventory' -count=1 -v`: ten tests passed, 5.493 seconds.
- `go test -race ./internal/inventory ./internal/httpapi`: passed (HTTP package 1.059 seconds); `go vet ./internal/inventory ./internal/httpapi`: passed.
- 1,000 repositories across ten scan pages and ten asynchronous import batches; scoped pagination, literal search, SSE scope, session revocation and cross-tenant RLS.
- 100 competing claims, process replacement, stale fences and rotation between read and persist.
- Partial inventory/cache failures, identity-preserving rename, complete-scan absence, team-version/current-owner checks, terminal-access pause and explicit retry.
- Signed GitHub/GitLab intake, duplicate IDs and digests, out-of-order and in-flight events, secret rotation/revocation, missed-event polling.
- Durable scheduler restart fairness and rate-limit backoff.

Coordinator additionally owns the real local Gitea private-supervisor integration test, including inventory, import and change reconciliation through the enrolled fixed-operation gateway. That result must be recorded separately from these database/adapter fixtures. Customer GHES/GitLab/private TLS topologies, production rate envelopes, cross-instance broker routing and 10,000-repository sustained-load qualification remain deployment gates.
