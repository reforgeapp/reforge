# T07.3 — outbound private connector

The connector delivers typed provider operations to an enrolled trusted supervisor. The supervisor constructs the fixed-origin client and contacts the private provider. The control plane does not dial that endpoint. Repository sandboxes receive neither the grant nor its credentials.

## Interfaces

- `runner.Service.AuthenticateSupervisor(ctx, token)` performs a fresh, two-second tenant-scoped read of the credential hash, runner state/expiry/version and active pool. It uses constant-time hash comparison and acquires no organization mutation lock or runner row lock.
- `privateconnector.New(Config{Authenticate, TTL, MaxConcurrent, Development})` creates a bounded, in-memory coordinator.
- `Dispatch(ctx, Target, Operation, Authorize)` waits for the exact organization's runner to poll before invoking the trusted authorization callback.
- `Authorize(ctx, Ready, Deliver)` must verify current authority and call `Deliver(GrantSpec)` synchronously while holding the required mutation locks. `Ready` includes the refreshed runner identity/version and SHA-256 credential hash. `GrantSpec` binds that exact version/hash, operation ID, persisted authority record ID and resolved connection.
- `httpapi.Server.RegisterPrivateConnector` registers `POST /runner/v1/private/poll` and `POST /runner/v1/private/results`.
- `privateconnector.NewClient(ClientConfig)` creates the separate supervisor client. `RunOnce` polls, executes one grant and submits its result. It performs no automatic provider retries. The runner's existing enrollment client supplies the supervisor identity and token.

Poll accepts only an empty JSON object. There is no connection selector, URL, HTTP method, arbitrary request body, or operation-dispatch endpoint for the supervisor. Registered `forge.*` reads cover GitHub, GitLab and Gitea: probe, inventory, repository lookup, ref resolution, immutable file read, change read/reconciliation, checks and approvals. `model.probe` and `model.list` cover the four direct API families; they never invoke inference. Inputs are exact typed argument structures. Merge, branch writes, model inference and unknown enums remain unsupported.

The authorization callback resolves endpoint, CA, approved private route, connection/credential versions and secret from server state. Grant validation binds organization, runner and connection IDs to that route. The executor creates the existing DNS-pinned, redirect-denying `network.Client` from these values and invokes the configured adapter. GitHub App authentication remains native installation-token authentication; no private GraphQL writes are registered. TLS is required; explicit development mode permits plaintext only on a loopback IP. The supervisor cannot enable development mode through a grant.

## Authorization and durable effects

Readiness is established before the authorization transaction starts. Immediately before calling `Authorize`, the connector refreshes supervisor authentication using the snapshot-read method; the callback must additionally validate the current runner/pool binding, credential version, scoped repository, current session/job lease/fence, policy, connection/route versions and operation-specific authority under its transaction locks. Call `runner.Service.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash)` within that transaction to lock and compare the current runner/pool state, version, credential hash and expiry. Set `GrantSpec.RunnerVersion` and `CredentialHash` only after this validation. Delivery and completion perform no additional database acquisition.

A result uses an unpredictable one-use capability plus the captured supervisor credential hash, bound to the grant's organization, runner, operation and expiry. Completion does not acquire database locks. This permits the callback to keep mutation locks while waiting for the native result: revocation waits, and the result can complete the transaction. A runner's next poll waits until its current authorization finishes.

Before future mutating or paid dispatches, the controller must commit its durable dispatch-attempt marker and required budget `MarkDispatched` record before entering the authorization transaction that can release credentials. A marker written only inside that later transaction can roll back after the provider has acted. The callback must refer to the exact persisted operation and record the result or conservative unknown outcome; the connector does not invent durable authority. Read-only capability/inventory requests can use their scoped audit request identity without a workflow reservation.

After credential delivery starts, timeout, cancellation, callback transaction failure or process loss is uncertain. The connector does not replay the operation. The controller must reconcile native state and retain uncertain provider charges/reservations according to T05. Registered handlers currently perform reads only; adding mutation/model handlers requires their typed authorization, native guards and accounting contracts.

Default poll/authorization TTL is ten seconds; the hard maximum is thirty seconds. The supervisor also derives a conservative monotonic execution deadline from the start of its poll and the granted remaining duration, preventing a slow supervisor clock from extending execution. Expired grants do not execute. Provider calls inherit that deadline. Results are capped at 6 MiB total, allowing a 4 MiB decoded source file; grants are capped at 1 MiB. Unsupported or malformed responses fail closed.

Ordinary JSON, formatted values and structured logs redact connection secrets, supervisor credentials and result capabilities. Only the explicit grant wire serializer includes the secret and capability for the authenticated supervisor. Provider errors use bounded codes; known credential echoes, including encoded values and file contents, are rejected. Grants and credentials are never written to a connector database, outbox, log or environment variable. Go string copies remain subject to normal process-memory lifetime.

## Topology and capacity

The coordinator is process-local. Controller dispatch, supervisor poll and result submission must reach the same instance through a dedicated controller endpoint or stable affinity. Arbitrary load balancing across API replicas is unsupported: it cannot recover another process's pending grant. A restart loses in-memory readiness/results; durable attempt records must drive reconciliation before retry. This component does not provide HA claims or durable secret storage.

Only one operation per runner is active. The default concurrent readiness/dispatch limit is eight; configure it below available controller database pool capacity. Delivered operation IDs remain in a bounded 10,000-entry replay cache for five minutes. Expired entries are pruned before admission; capacity exhaustion within that window fails closed. Durable outbox authorization supplies cross-restart and long-term replay protection for future writes. Delivery and result completion still work when authorization owns a transaction and the rest of the database pool is saturated.

## Validation

The real fixture uses the disposable `reforge_test` database and local Gitea 1.27.3. It performs actual enrollment, starts a separate supervisor process with a clean environment and passes its credential over stdin. That process executes the real Gitea probe and inventory calls through outbound long polling. No paid providers are called.

The race-enabled suite verifies ready-before-authorization ordering, one-use result capabilities, wrong credentials, revocation after readiness, wrong runner/tenant binding, arbitrary operation/proxy rejection, timeout/server-loss uncertainty, duplicate operation rejection, secret formatting, and a complete 4 MiB file result over HTTP. Real PostgreSQL tests hold organization/runner/connection locks during delivery, prove result completion avoids deadlock, verify concurrent revocation waits for authorization commit, and complete a result with every database connection occupied. More than 10,000 sequential dispatches prove expired tombstones release capacity while live tombstones reject replay.

```sh
set -a
source .local/development.env
set +a
REFORGE_PRIVATE_GITEA_TEST=1 REFORGE_TEST_ROOT=/home/mnorris/repos/reforge GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go test -race -count=1 ./internal/privateconnector
```

Root integration owns startup registration, runner command coordination, durable intent/budget authorization, connection factories and later model/provider handlers. Hosted sandbox qualification and external G5 remain open.


## Integrated onboarding and read gateway

`cmd/runner` now implements enrollment and the private connector. Create an empty onboarding pool in the GUI, request a one-use enrollment token, and save it to a private regular file with mode0600. Empty pools authorize no repository jobs. Run:

```sh
bin/reforge-runner enroll --endpoint https://reforge.example --credentials /private/runner.json --token-file /private/enrollment.token --name customer-worker
bin/reforge-runner connector --endpoint https://reforge.example --credentials /private/runner.json
```

Add `--ca-file /private/controller-ca.pem` for a custom controller CA. Plaintext loopback development additionally requires `--development`; it cannot enable remote plaintext. Enrollment rejects a symlink or group/world-readable token file. Credential persistence is atomic/private; the running connector rotates expiring credentials and stops on revocation. The repair processor is separate T19 work.

`providers.Service.Read` waits for private readiness before opening its tenant transaction. It then locks the organisation, checks current connection/version and active runner/pool credentials, invokes the caller's fresh scope/job-fence callback, resolves the encrypted secret and issues exactly one typed read. Public reads use the same gateway and approved destination transport. Callers separately fence result persistence against connection changes. `providers.DecodeWebhook` invokes signature verification without network access or forge-token resolution.

Real integration against disposable PostgreSQL, enrolled Gitea1.27.3 and Ollama0.34.2 passed3.777s under the race detector: initial private creation, encrypted credential resolution, browser HTTP capability probe, fixed inventory and canonical change reconciliation reads on a disposable repository, callback rejection before grant delivery, empty pool job denial, secretless private model metadata, and revocation. The model metadata probe is not inference certification. Generic connector tests passed9.290s under race; no general URL/proxy or paid operation was added.
