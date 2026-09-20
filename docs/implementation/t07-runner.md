# T07 runner authority and artifacts

Status: enrollment, scoped job credentials, controller boundaries and private local artifacts are implemented and tested against PostgreSQL 18.6. The coordinator owns the isolated runtime, runner process, startup configuration and migrations. This service does not certify a hostile-repository sandbox or live private-provider connectivity.

## Construction

`runner.New(store, identity, workflow, artifacts)` constructs the authority service. `artifact.NewLocal(store, absoluteDirectory)` opens a private local backend; the directory must have mode 0700 and must not be a symlink. Close the backend during shutdown.

Register `runner.CheckRunnerTx` with `connections.RegisterRunnerCheck`; register `runner.CheckScopeTx` for workflow runner-pool existence checks. Campaign checks still require their owning service. `Server.RegisterRunner(service)` mounts browser and supervisor routes. The schema proposal is `internal/runner/schema.sql`; coordinator migration009 additionally binds task/pool, task/repository and attempt/task composite keys.

The artifact backend exposes transaction-aware `PutTx` and `MetadataTx` plus verified `Open`. It does not implement the older unfenced `ArtifactStore.PutTenantArtifact` adapter. Controller writes must already hold the current tenant, runner and workflow fence transaction. Browser calls go through the runner service's fresh-session metadata/download methods.

## Enrollment and credentials

Owners create versioned pools with explicit repository grants. An owner with restricted repository scope must cover the pool's existing and requested repositories. A pool state is active, draining or revoked; revoked pools cannot be reactivated. Pool changes invalidate existing job credentials and permanently fence affected attempts, so removing and restoring a grant cannot revive a lease. Draining conservatively revokes active job authority as well as blocking claims.

Enrollment tokens expire after 15 minutes and are consumed once. Any pool update invalidates its outstanding enrollment tokens. Consumption rechecks the issuing owner’s current role and effective direct/team repository scope; normal browser logout does not revoke a delegated setup token. A successful enrollment returns a supervisor credential valid for 24 hours. Tokens contain routing IDs and 32 random bytes; only SHA-256 hashes are stored, with constant-time comparison. Every lookup enters the token's tenant RLS context and authenticates its hash; no global credential lookup bypass is used. Enrollment grants, consumption, pool edits, rotation and owner revocation are audited without secrets.

Supervisor rotation immediately replaces the credential, revokes job credentials and invalidates active leases. Rotate while idle. A lost rotation response requires owner-issued enrollment rather than replaying an old credential. Owner revocation requires the current runner version and serializes with in-flight operations through the organisation lock. Revocation proves loss of authority; it does not prove a remote process stopped.

Each claim receives a separate job credential binding organisation, repository, pool, runner, task, job, attempt, operation ID, fence, policy hash, expiry and a server-defined method allowlist. Claims select only exact task pool matches and currently assigned repositories before selecting a candidate. Unassigned tasks are not dispatched. A supervisor has at most one live assignment. Neither a request body nor a job credential can supply additional tenant/repository grants.

Job credentials expire after 5 minutes, bounded by supervisor expiry; successful heartbeats renew them with a 1-minute lease. Every job action checks current runner/pool state, repository grant, credential hash/expiry/method and the database-clock lease/fence under the same transaction. Policy drift durably revokes the assignment after rolling back the rejected operation. Credential types cannot substitute for one another or authenticate browser endpoints.

## HTTP protocol

All `/runner/v1` requests use `Authorization: Bearer <credential>`, the configured control-plane Host and no browser Origin. Responses containing credentials have `Cache-Control: no-store`. Credentials never appear in query strings, ordinary DTO JSON or logging representations.

| Route | Credential and input | Result |
|---|---|---|
| `POST /runner/v1/enroll` | Enrollment; `{name}` | Supervisor token, expiry and runner |
| `POST /runner/v1/rotate` | Supervisor | Replacement supervisor credential |
| `POST /runner/v1/claim` | Supervisor; no grants | Job token, expiry, lease and task; 204 when none |
| `POST /runner/v1/heartbeat` | Job | Renewed lease, or `stop:true` for requested cancellation |
| `POST /runner/v1/progress` | Job; `{state}` | Fenced lifecycle transition |
| `POST /runner/v1/result` | Job; completion outcome | Fenced result; credential revoked afterward |
| `POST /runner/v1/artifacts` | Job; raw body, Content-Type and X-Artifact-Name | Artifact metadata |
| `GET /runner/v1/artifacts/{id}` | Job | Attachment from that task/repository |
| `POST /runner/v1/operations/{name}` | Job; `{connection_id,input}` | Registered fixed operation only |

On lost credentials, revoked authority, expired fence or policy rejection, the supervisor must stop its local work and descendants. Cancellation heartbeat reports a request; only a runner-confirmed cancelled result records a confirmed stop. Unknown external outcomes remain reconciliation work. These endpoints do not confer native provider publication authority. T19 must provide trustworthy validation evidence before repair completion becomes a production workflow.

Browser owner routes under `/api/v1/orgs/{orgID}` provide GET/POST runner-pools, PUT runner-pools/{poolID}, POST runner-pools/{poolID}/enrollments, GET runner-pools/{poolID}/runners and DELETE runners/{runnerID}. Updates/revocation use If-Match. Pool and runner lists accept limit/cursor and return domain.Page with explicit complete/next_cursor fields; filtering precedes pagination. Browser session/CSRF/Host checks are shared with identity. GET artifacts/{id} returns freshly scoped metadata; GET artifacts/{id}/download returns an attachment.

## Private operation boundary

`RegisterOperation(name, FixedOperation)` registers an explicit trusted controller handler. No operations are enabled by default. The broker rejects arbitrary operation names and validates the enrolled runner, exact task pool/repository, current connection route and its connection-to-repository or task-model binding. Handler invocation holds current authority/fence locks and has a 10-second deadline. Revocation waits for this bounded invocation; later calls fail.

A registered handler must validate a fixed typed input, enforce that operation's policy/provider prerequisites and return sanitized bounded output. It must dispatch a qualified operation through the outbound enrolled runner. It must not dial a private endpoint from the hosted control plane, return credentials, accept an arbitrary URL/method or expose a generic network proxy. The fixture test exercises authority and transaction ordering only; it performs no provider call and makes no successful-connectivity claim. Actual outbound private transport qualification remains required.

## Artifact storage and lifecycle

Uploads accept UTF-8 text/plain, text/x-diff and valid application/json. Limits are 1 MiB per artifact, 256 MiB and 4096 records per tenant, with 30-day maximum retention. Bodies are buffered with a 10-second HTTP read deadline before acquiring authority locks; the fresh fence check and metadata insertion happen after capture. Files use server-generated tenant/artifact UUID names, mode 0600, rooted filesystem operations and no-follow/exclusive creation. Names cannot select paths; binary/control-byte content and common authorization/token/private-key material are rejected. This detector is not a guarantee that arbitrary unlabeled secrets can be recognized: capture must keep known credentials out of repository output before upload.

Metadata binds the task and attempt in the same tenant transaction. Failed transactions remove their blobs when the caller remains alive. A crash can leave an orphan; `SweepOrphans` removes up to 200 old unreferenced blobs per call, with a minimum one-hour age and bounded directory batches. `DeleteByRetention` removes up to 200 expired artifacts per tenant call. Startup/maintenance scheduling is coordinator-owned; repeat until the pending batch is drained. Operator storage quotas remain necessary for the aggregate volume across tenants.

Downloads verify file type, size and SHA-256 and return application/octet-stream attachments with nosniff. Browser downloads refresh session/membership/repository authority on each 32 KiB read. Worker downloads refresh credential, repository/task binding and the live fence on every read. Expired/deleted artifacts and inaccessible IDs do not reveal metadata. Stored content remains untrusted; no HTML or script rendering is enabled by these endpoints.

## Evidence

Real PostgreSQL race tests cover 20 competing one-use enrollments, expired enrollment/job credentials, issuer role/scope/team revocation, pool expansion invalidation, delegated setup after logout, accurate inventory pagination, rotation and revocation, supervisor/job separation, repository/pool filtering before selection, durable fence invalidation, cancellation delivery/confirmation, policy drift, method allowlists, scoped-owner restrictions, RLS denial, mid-download membership/lease revocation, retention/orphan deletion and fixed-operation/revocation serialization. Local storage tests cover credential rejection, size limits, path/symlink attacks, file permissions and digest corruption. HTTP tests cover browser-origin rejection, separate browser authority and absence of an arbitrary proxy.

No paid API, live private provider, external publication or production sandbox certification was used for these checks.

Verification: all 11 runner tests passed with `go test -race -count=1 ./internal/runner -v` against PostgreSQL 18.6 (2.489s). Artifact race tests passed (1.046s); all six affected workflow race tests passed (2.514s). `go vet ./internal/runner ./internal/artifact ./internal/httpapi` and `git diff --check` passed.
