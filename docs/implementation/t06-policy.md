# T06 policy integration

`internal/policy` implements immutable policy versions, scoped activation and deterministic evaluation. Root installs `internal/policy/schema.sql` as migration 004. Runtime RLS permits version inserts/reads; existing versions cannot be updated/deleted. Activation updates a binding and writes `policy.changed` to `audit_events` in one transaction.

## Service and HTTP wiring

Construct `policy.New(store, auth, deploymentPolicy)`. Deployment policy is trusted operator configuration, never a browser field. `{"schema":"maintenance/v1"}` adds no operator constraints; missing organisation policy still blocks automation. Production operators should supply explicit approved recipes/models/routes and resource ceilings. The evaluator always requires action-specific execution, validation, provider and provenance evidence independently of optional policy rules.

| HTTP shape | Service call |
| --- | --- |
| `POST /api/v1/orgs/:orgID/policies/versions` with `{scope,policy,reason}` | `CreateVersion(ctx,session,orgID,scope,policy,reason,requestID)` |
| `GET /api/v1/orgs/:orgID/policies/versions/:versionID` | `GetVersion(ctx,session,orgID,versionID)` |
| `GET /api/v1/orgs/:orgID/repositories/:repoID/policy` | `Resolve(ctx,session,orgID,repoID)` |
| `POST /api/v1/orgs/:orgID/policies/versions/:versionID/simulate` with `{repository_id,primary_team_id,input}` | `Simulate(ctx,session,orgID,versionID,repoID,primaryTeamID,input)` |
| `POST /api/v1/orgs/:orgID/policies/versions/:versionID/activate` with `{repository_id,primary_team_id,simulation_hash,reason}`, quoted `If-Match` binding version | `Activate(ctx,session,orgID,versionID,repoID,primaryTeamID,expected,simulationHash,reason,requestID)` |

Use existing identity/CSRF middleware and structured errors. Return new binding version as ETag; version zero means no existing binding. Root owns transport/OpenAPI updates. Initial organisation activation permits an empty repository ID. Team policy simulation needs a repository currently bound to that team; repository policy uses its own repository ID. Simulations perform no writes. The simulation hash covers the proposed policy/version and current governing bindings; activation recomputes it under the organisation mutation lock. A simulation may show denied/unknown actions: administrators may intentionally activate restrictive policies.

Owner controls organisation policy; owner/admin controls authorised teams/repositories. Candidate files have no activation path. Import creates only an inactive immutable version. Defaults use repository, explicitly designated primary team, then organisation; every bound team's constraints still apply.

## Controller contract

Within a tenant transaction, call `ResolveTx(ctx,tx,orgID,repoID)`, collect trusted current facts, then `EvaluateTx(ctx,tx,orgID,repoID,input)` immediately before the mutation boundary. `EvaluateTx` reloads governing policy and supplies server time. UI-supplied evidence is for simulation only. Controllers must use the same organisation locking order as `auth.WithMutation` when serializing local mutations against activation/revocation; provider execution races require the later certified adapters.

`Input.Current` binds exact head, target, tested revision, effective policy hash, provider-rules/capability versions and deployment source/artifact. Evidence must match that tuple, carry a reference and be fresh. `StartingPolicyHash` is retained for attribution and grants no ongoing authority. Native identities belong in requirement/evidence `identity`; same-named checks from another publisher fail. Approval counts must come from verified, distinct eligible identities in trusted adapters.

All actions require `execution_authority`. Repair also needs `budget_capacity`; publication needs validation, branch ownership and exact-head enforcement; merge additionally requires native rules/reviews/checks, target enforcement and merge cooperation; deployment/recovery require native approvals and artifact/route authority. These gates are assertions from trusted controllers/adapters, not provider implementations. Only `allow` authorises dispatch. Unsupported, absent, duplicate, expired or mismatched evidence blocks.

Policy ceilings are non-negative integer units. `budget` uses the controller's configured accounting unit; it is not a currency conversion. T05 must atomically reserve every applicable scope's independent budget and report `budget_capacity`. Resource usage includes the proposed operation. `null`/omitted allowlist inherits; `[]` denies all. Numeric zero is a real ceiling. Effective requirements retain separate per-action rows when counts/identities differ; layers preserve source rules and binding versions.

Org/repository pause, archive/access state and activated policy pause are loaded from PostgreSQL. T05 supplies applicable recipe/campaign/model-connection/runner-pool pauses in `Input.PausedScopes`. T05 also consumes activation audit changes into its unified outbox/event stream, invalidates queued decisions and handles cancellation/reconciliation. This package does not claim to revoke a provider operation already past its commit point.

## Validation

`go test ./internal/policy` runs the deterministic policy corpus. Set guarded `REFORGE_TEST_DATABASE_URL` targeting exactly `reforge_test` to include real PostgreSQL activation, scope, immutability, stale-version and session-revocation checks. Provider-specific guards, queue/train enforcement, bot reconciliation, validation-plan integrity and deployment attribution remain adapter/controller responsibilities; the policy corpus verifies their failed/unknown/stale evidence blocks actions.

Root integration: server reads optional `REFORGE_POLICY_FILE` as a strict JSON deployment policy. GET `/policies/effective?repository_id=...` and paginated GET `/policies/versions?scope_kind=...&scope_id=...` provide effective state/history. Policy budget values use integer microUSD alongside T05's separate token/time/request caps. HTTP activation, stale version, simulation and CSRF checks passed against PostgreSQL18.6.
