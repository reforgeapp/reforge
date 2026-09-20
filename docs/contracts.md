# Domain and API contracts

Updated: 2026-09-21. Contract baseline to formalise in OpenAPI and Go during T01. T31 profile rules apply.

## Custom command and agent profile contract

T31 profiles are administrator-approved, versioned records. Each immutable version binds executable identity, fixed argv template, container image digest, protocol version, declared input/events/output/cancel/exit/usage semantics, approved runtime/version, entitlement evidence, policy binding and allowed run types. Tenant request data cannot provide arbitrary executable paths, shell text or image tags. Events preserve unknown usage and uncertain cancellation/exit outcomes. Secret custody stays outside command input/events/logs; approval occurs before effects.

Agent identities remain distinct: Claude Code, Codex and Google Antigravity CLI `agy` (not Gemini CLI). `agy` headless JSON/NDJSON behavior is a technical qualification target only; authentication does not prove subscription entitlement, isolation or approval safety. Runtime enablement requires entitlement, headless, container topology and pre-effect approval evidence for exact version/topology. No subscription token becomes provider API key and no silent fallback occurs.

## Records

Use UUID identifiers, UTC timestamps, explicit version counters and JSON schemas for recipe-specific payloads. Provider-native identifiers remain opaque strings; GitLab numeric IDs must not be confused with globally unique IDs.

| Record | Key fields and constraints |
| --- | --- |
| Organisation | name, settings_version, pause_state, budget currency/unit, retention policy |
| Membership | org_id, user_id, role, team scopes; unique within org |
| Team | org_id, name, scoped repository bindings |
| ForgeConnection | org_id, provider, instance_url, immutable instance/installation identity, secret_ref, capabilities, server_version, last_verified_at |
| Repository | org_id, connection_id, native_id, current_full_name, default_branch, archived, team labels, policy_binding, last_synced_at; unique `(org_id, connection_id, native_id)` |
| ModelConnection | org_id, provider/protocol, endpoint, secret_ref or agent_identity_ref, capabilities, entitlement_state, verified_at |
| ModelProfile | connection_id, model, role, tool constraints, max_input/output, max_duration, fallback policy, version |
| RunnerPool / Runner | org_id or explicitly shared control identity, allowed tenants, trust level, private routes, capabilities, enrolment/expiry, heartbeat |
| PolicyVersion | immutable canonical policy, hash, author, effective scope, parent bindings, created_at |
| RepositoryBaseline | repo_id, commit_sha, detected stack, bot config/ownership, commands, check results, missing capabilities |
| Finding | source identity, fingerprint, category, severity, evidence refs, owner, status, snooze expiry, first/last_seen, revision |
| Task | finding_id, recipe/version, target ref, policy_version, budget reservation, priority, state, cancellation_version, idempotency_key |
| Attempt | task_id, number, fencing token, runner, base/head SHAs, model profile, start/end/outcome |
| Validation | attempt_id, baseline_sha, candidate_sha, target_sha, pinned validation_plan_id/hash, command/config/check identities, outcome, artifact refs, trust level |
| Change | repo_id, native PR/MR id, branch, head/base SHAs, ownership `app/bot/human`, originating bot_change_id, expected_effect, state |
| GateEvaluation | change_id, head_sha, target_sha, policy_hash, provider_rules_hash, evaluated_at, blockers[], required_actions[] |
| Deployment | change_id, merge_sha, workflow/environment/artifact identity, provider run id, correlation id, approvals, state, health_evidence |
| Campaign | pinned repo selection, recipe_version, canary count, concurrency, stop criteria, member task IDs |
| UsageLedger | org/repo/task/connection, reserved/actual amount, unit, source `provider/reported/estimated`, timestamp |
| AuditEvent | actor, tenant, action, object, before/after version refs, decision/evidence refs, request/correlation id |
| Job / Outbox | idempotency key, tenant, stage, lease owner/expiry, fencing token, payload schema/version, attempts |

No raw access token, API key or login cookie appears in business records, logs, events or artifacts. Secret values go through write-only connection setup and reference-based execution.

## Identity and deduplication

- Findings: provider immutable source ID where available; otherwise versioned fingerprint of repo ID, category, manifest/path/symbol, advisory/error signature and affected range. Store algorithm version to avoid accidental duplicates after upgrades.
- Dependency work: include ecosystem, manifest, package group and requested version/range; detect grouped and superseding bot updates. A bot author name alone is not trustworthy ownership proof.
- Operation IDs are unique by tenant and intended external action. DB unique constraints and provider reconciliation complement each other.
- A new task attempt can reuse the finding but never reuse stale verification. A new commit invalidates candidate approval even if the title is unchanged.

## API conventions

`/api/v1/orgs/{orgID}/...`; authenticated membership plus applicable team/repository permission required for every handler and descendant object. SSE/artifacts/exports use the same scope resolver. Explicit connection IDs disambiguate identical repo names across instances. Error response: `{code, message, request_id, retryable, details}`. Never expose an inaccessible object's existence through detail messages.

List endpoints accept cursor/limit and allowlisted filters/sorts. Default 50, maximum 200. All times RFC3339; money stored as integer minor units or fixed-precision decimal with explicit currency. Provider credits/tokens are separate units, not fabricated dollar equivalents.

Mutations accept `Idempotency-Key`; policy/config edits require version/`If-Match` and return conflict if stale. A merge request includes the exact gate-evaluation ID and expected head/target SHA; the server still re-evaluates.

| Endpoint family | Operations |
| --- | --- |
| `/session`, `/organisations` | identity, memberships, organisation switching; OIDC callback separate |
| `/connections/forges` | create, test, list capabilities, configure webhook, rotate/revoke |
| `/connections/models` | create, test, discover/enter models, inspect billing route, rotate/revoke |
| `/repositories` | paginated inventory, sync/import previews, import selection, pause/resume, baseline |
| `/findings` | list/detail, queue, assign, dismiss/snooze/reopen with reason |
| `/tasks` | list/detail, plan, attempts, events, cancel, retry-from-safe-stage |
| `/changes` | list/detail, diff/artifacts, refresh gates, native review link, request eligible merge |
| `/policies` | immutable versions, simulation, bindings, rollout preview, apply/revert |
| `/campaigns` | preview pinned selection, start/pause/cancel, progress and stopped reasons |
| `/deployments` | list/detail, request allowlisted pipeline, gate status, cancel, request approved recovery |
| `/runner-pools` | enrol/revoke, capabilities, route probes, drain; one-time enrolment credentials |
| `/usage`, `/audit` | aggregate reports, pagination/export, budget configuration |
| `/events` | SSE subscription, filters and Last-Event-ID replay within retention |
| `/artifacts/{id}` | authorised metadata/download; no raw filesystem path |

Webhook endpoints are connection-bound and provider-specific outside ordinary browser auth. Worker protocol is separate `/runner/v1` authenticated with short-lived runner credentials; workers cannot call browser administrative endpoints.

Supervisor enrolment is distinct from a job credential. Each job credential binds org/repo/job/attempt/fence/expiry and a method allowlist. Server derives these values from the live lease, never trusts client IDs. Apply checks to heartbeats, progress, artifacts, result submission and model-broker calls. Runners never connect to PostgreSQL.

## Events

Envelope: `id, org_id, type, aggregate_type, aggregate_id, aggregate_version, occurred_at, request_id, data_version, data`.

Events include `repository.synced`, `finding.upserted`, `task.queued`, `attempt.started`, `attempt.progress`, `validation.completed`, `change.published`, `gate.changed`, `merge.reconciling`, `change.merged`, `deployment.changed`, `budget.paused`, `connection.degraded`, `policy.changed`, `automation.paused`.

SSE is a view over durable events, not the job bus. Reconnect replays authorised events; expired cursors instruct the client to refetch. Membership revocation terminates subscriptions. Progress describes observed stages/tool results; do not manufacture percentage complete from model prose.

## Go interfaces to freeze at T01

Use concrete request/result structs and small interfaces; avoid a generic map-based plugin bus. All methods accept `context.Context`, tenant-bound connection handles and typed errors.

| Interface | Required operations |
| --- | --- |
| ForgeInventory | ProbeCapabilities, ListRepositories, GetRepository, ReadFileAtRef, ResolveRef |
| ForgeEvents | VerifyWebhook, DecodeEvent, ReconcileChanges, ListChecks, ListBotWork |
| ForgeChanges | CreateChange, FindChangeByOperation, UpdateAppBranch, ReadChange, RequestReview |
| ForgeProtection | ReadEffectiveRules, EvaluateNativeEligibility, ReadApprovals, ReadQueueState |
| ForgeMerge | RequestNativeMergeOrQueue, ReadMergeResult; expected head, target and gate snapshot required |
| ForgeDelivery | ListAllowedWorkflows, TriggerOrObservePipeline, ReadDeploymentGates, ReadDeploymentStatus, RequestAllowedRecovery |
| ModelProvider | Probe, ListModels when supported, StreamTurn, Count/EstimateUsage, Cancel |
| AgentExecutor | ProbeVersionAndAuth, Start, StreamEvents, DecideApproval, DelegateIsolatedCommand, ResumeIfSupported, Cancel; executable/version allowlist and required pre-execution interception capability |
| SandboxRuntime | PreparePinnedWorkspace, ExecuteBoundedCommand, ApplyPatch, CollectArtifact, Destroy |
| PolicyEvaluator | ResolveEffective, EvaluateAction, Explain, Simulate; no LLM dependency |
| ArtifactStore | PutTenantArtifact, GetMetadata, AuthoriseDownload, DeleteByRetention |

`Capability` includes state `supported/unsupported/unknown`, scope, reason, source/version and last_checked. Absence of a capability is never interpreted as permission. `ProviderError` classifies auth, scope, rate limit, unsupported, transient, conflict, policy rejection and uncertain side-effect outcome.

Model stream events normalise text, tool-call IDs/arguments, completion, usage and provider errors. Buffer fragmented arguments and validate against the tool schema before executing once. Preserve provider-required opaque continuation fields internally; never display them as evidence. Unsupported tools/invalid JSON stop or retry a bounded turn without shell execution.

## Policy representation

Versioned JSON canonical form, optional YAML import/export. GUI writes the same schema. v1 uses typed conditions and explicit deny rules, not arbitrary scripts or a new policy language. Example shape:

```yaml
schema: maintenance/v1
scope:
  team: platform
recipes: [dependency-repair, build-repair]
limits:
  max_open_changes_per_repo: 3
  max_attempts_per_task: 2
  max_changed_files: 12
  max_changed_lines: 400
  monthly_api_budget:
    currency: USD
    amount: "100.00"
models:
  repair_profile: team-approved-coder
  allow_paid_fallback: false
changes:
  branch_prefix: maintenance/
  forbidden_paths: [".github/workflows/**", ".gitlab-ci.yml", ".gitea/workflows/**", "infra/prod/**"]
dependencies:
  mode: cooperate
  bot_branch_writes: false
merge:
  mode: policy
  require_native_rules_verified: true
  require_up_to_date_evidence: true
  allowed_risk_classes: [low]
deploy:
  mode: existing-pipeline
  environments: [staging]
  allowed_workflow_ids: [staging-release]
  production_requires_native_approval: true
```

The displayed budget is an example, not a pricing claim. Hosted operator hard limits and organisation denies cannot be loosened by repo policy. A policy import from a PR is proposed configuration only until approved by a policy administrator.

## Direct-model continuation and tool validation

`Event.Continuation` is provider-opaque complete replay history through the finished turn. A subsequent `TurnRequest` carries that continuation plus only new messages; system instructions and tool definitions are supplied each time. Adapters preserve native reasoning/signature items and never flatten them into model-visible transcript text. The repair controller buffers tool calls until the whole turn succeeds and validates them again at the execution boundary.

Shared `model.CompileTools` validates bounded JSON Schema definitions without network/file resolution. Final arguments must pass the registered schema. Positive output limits, explicit connection/model binding and no automatic paid retries are mandatory. Provider failure/cancellation after dispatch preserves uncertain usage until authoritative accounting arrives. Byte estimates are planning data; the budget broker reserves a configured context/output ceiling, including opaque continuation uncertainty.

Normalized model input usage includes all input tokens; `CacheTokens` is the cache-read subset, and `CacheCreationTokens` separately identifies cache-write input. Provider-specific premium cache writes must be priced explicitly or reserved at a conservative ceiling. Output usage includes billable hidden reasoning when the provider reports it separately.
