# T05 durable workflow service

Status: locally implemented and tested with PostgreSQL 18.6. Runner authentication, real sandbox shutdown and live provider reconciliation remain later-ticket qualification work.

`workflow.New(store, identity, PolicyCheck)` constructs the service. The policy callback runs inside the caller's tenant transaction and returns the currently permitted effective policy hash. Missing callbacks, callback errors, empty hashes and changed pinned hashes fail closed. Enqueue pins the policy, recipe/version, repository/target branch, model connection and named model route. The original policy hash remains recorded after an explicitly authorised resume refreshes current policy.

## Browser and controller boundaries

Browser services: `Enqueue`, `Get`, `List`, `Cancel`, `Resume`, `SetPause`, `Replay`. Mutations use `auth.WithMutation`, refresh current membership, enforce repository scope/role and append actor/version/request audit records in the same transaction. Cancellation/resume/pause require the expected version. Enqueue uses a tenant-scoped idempotency key and immutable input hash; a changed request cannot reuse that key. Model-route selection defaults to `default` and remains immutable on the task.

Controller-only services: `Claim`, `Heartbeat`, `Advance`, `Complete`, `Recover`, `ValidateFenceTx`, outbox methods and retention maintenance. Do not mount these as browser APIs. T07 must derive worker ID and allowed organisation IDs from enrolled, authenticated runner assignment; request IDs alone never establish authority. `RegisterScopeCheck` installs trusted campaign/runner-pool existence validation. Those scopes fail closed before their owning services register the check; model scopes must resolve to a model/agent connection in the same tenant.

## Leases, attempts and recovery

Claim selection rotates organisations using a narrow RLS-protected scheduler table containing organisation IDs and timestamps, then enters a tenant transaction. Within an organisation it prefers the least-recently selected repository, then priority and task creation order. Paused or unavailable scopes do not block unrelated repositories. One writer occupies each repository/target branch; unknown external outcomes retain that exclusion until reconciliation. Scheduler selection is separate from job dispatch, so a scheduler crash can delay fairness rotation without issuing a lease.

A claim creates a persisted Attempt, increments the job fence and lease-owner binding, and starts reproduction. Heartbeats compare database time, owner, fence, task, attempt, repository and operation identity. Retries create a new attempt and advance the fence while retaining the logical job operation ID. Task progression enforces reproduction → planning → repair → validation → publication → completion. Failed attempts use bounded backoff; attempt ceilings cannot be bypassed by resume. Lease expiry before mutation can retry; publication, unresolved outbox dispatch or cancellation uncertainty enters reconciliation. Recovery processes at most 200 expired leases per tenant transaction and can be polled again.

All privileged controllers must call `ValidateFenceTx` inside the same transaction that holds the organisation/job/attempt locks and performs the bounded action. Validate the actual action, current policy and applicable pauses before composing budget, model-broker, artifact or native-action work. Budget reservation uses the returned server-derived task bindings and a distinct operation ID for each bounded allowance. `ValidateFenceTx` returns policy drift to its caller; controllers must persist a blocked transition after their failed transaction or use the service's fenced operations, which already commit the block. A failed transaction cannot also persist its own status update.

Pausing an organisation, repository, recipe, model, campaign or runner pool persists a versioned control, permanently revokes affected running fences and leaves tasks blocked or reconciling. Quickly unpausing cannot revive the old attempt. Resume requires current-policy authorisation and a new attempt. A revoked lease means authority was removed; it does not prove the old process stopped. Cancellation distinguishes pending cancellation, runner-confirmed stop and uncertain external outcome. Already completed provider actions remain recorded.

## Outbox invocation contract

`PrepareIntentTx` validates the fence and persists an idempotent intent plus event in one transaction. The operation UUID is stable across retries. Canonical JSON preserves numeric precision. `BeginIntent` commits `dispatching` before an external invocation; it records intent and is not permission to make a later unguarded call.

After `BeginIntent` commits, the trusted controller must open a tenant transaction, call `ValidateFenceTx` with the actual operation kind, keep the locks through a bounded provider invocation, and use the stored operation ID for provider deduplication/reconciliation. Never blindly invoke an already-dispatching intent after a crash. `CompleteIntent` records a live worker's observed outcome; `ReconcileIntent` is for trusted provider observers after worker loss. Unknown/dispatching outcomes block retry and task completion. A proven absent outcome permits a bounded redispatch; successful operations cannot be redispatched. A task cannot complete while any prepared external intent remains unfinished.

Delivery remains at least once; the service does not claim distributed exactly-once execution. Provider-native idempotency or canonical operation lookup is required before repeating an uncertain call. No provider adapters or external mutation are executed by this ticket.

## Event replay and SSE

Events and state commit together. A locked per-organisation counter preserves commit order; no cross-tenant global business-event scan is used. `Replay` refreshes session expiry/revocation and membership/bindings on every batch. Repository events use current repository grants; organisation-wide events are owner-only. A batch is bounded by its captured event head so concurrent appends cannot move events ahead of its internal cursor.

`EventPage.ScanAfter` is server-only (`json:"-"`): SSE loops advance it past filtered events without emitting hidden IDs/data/counts. Emit only returned authorised event IDs. Polling the next batch detects revoked membership/session and must close/reset the stream. Retention maintains an explicit floor; expired cursors return `ErrCursor` and require refetch. Root owns the HTTP/SSE transport and its real stream tests.

## Local evidence

The real PostgreSQL race suite exercises 100 competing claims for one job; duplicate enqueue/intent rejection; service restart after lease expiry; stale worker results; policy drift and resume history; lost outbox dispatch, canonical reconciliation and stable redispatch operation IDs; cancellation uncertainty; paused-repository exclusion; organisation fairness; bounded retries; permanent pause fence revocation; immutable model-route normalization; bounded recovery of 201 expired writers without bypassing the unrecovered writer; scoped/cross-tenant event replay; retention expiry; session/membership revocation; audit actor context; and unscoped RLS denial. Outbox tests include integers above JavaScript's exact-number range. No live provider or paid inference was used.

Migrations are coordinator-owned. The proposal lives in `internal/workflow/schema.sql`; migration005 additionally constrains job/attempt task consistency, and migration008 adds the immutable model-route name. T07/T19/T21/T23 must qualify actual runner stop, provider dispatch/reconciliation and operation-boundary fencing. T25 supplies real campaign records; this service does not fabricate campaign or runner-pool enrollment.

Verification: `go test -race -count=1 ./test/integration -run '^TestWorkflow' -v` passed all six tests against PostgreSQL 18.6 (2.303s). `go vet ./internal/workflow` and `git diff --check` passed.
