# T09 GitLab adapter

`internal/forge/gitlab` implements bounded REST v4 inventory, signed/token webhook verification, status publisher identity, MR reconciliation, guarded publication, protection inspection and native merge/train operations. Local fixtures pass. GitLab.com and Self-Managed live certification remain open.

## Authority and transport

`New(forge.Config)` requires an explicit connection token, endpoint and controlled HTTP client. Instance paths normalize to `/api/v4`; requests use `PRIVATE-TOKEN`, reject redirects and bound each request to 15 seconds and 4 MiB. The factory must supply tenant-approved SSRF transport; the controller must bound the whole operation. No token or proxy is read from the environment.

`WithBranchAuthorizer`, `WithChangeAuthorizer`, `WithReviewAuthorizer` and `WithMergeGuard` are mandatory before writes. Callbacks reload persisted tenant/repository authority, workflow fence, operation intent, pause/revocation state and current policy at each side-effect boundary. Merge guards also bind `GateID`, exact H/B/T, current rules and actual provider/version qualification. `WithCheckPublishers(map[name]actorID)` binds required statuses to approved immutable publishers; merge requires an operational-actor-bound policy status and a qualified native execution gate.

`WithProtectionReader(*Provider)` permits a separately approved inspection credential on the same tenant and exact API endpoint. Only GET inspection uses that credential. Current user, membership, branch push authority and mutations retain the operational token. Administrative, Maintainer, custom-role or direct protected-target push authority cannot become an automation bypass. Some approval endpoints require elevated read visibility and Premium/Ultimate; unavailable evidence blocks the affected action. [Approval APIs](https://docs.gitlab.com/api/merge_request_approvals/).

## Publication and evidence

Inventory supports projects and subgroup namespaces, immutable IDs, strictly advancing bounded pages and incomplete-response rejection. Provider timeouts, malformed mutation responses, HTTP 408 and server errors produce uncertain outcomes. Callers reconcile instead of blindly retrying. Owned MR lookup authenticates `/user` and binds actor, operation marker, source/target projects and branches; ordinary service users are valid owners. Duplicate matches conflict. Creation validates exact H, returned ownership and branches, and refreshes current B. Draft requests use the documented title convention. Review assignment requires its own fresh callback. No undocumented idempotency header is sent.

Fresh `reforge/` branch publication creates a unique operation-specific staging branch at BaseSHA, commits bounded edits without force, verifies the sole parent and complete resulting tree, then creates the final ref with native uniqueness. Every write repeats authorization. Publication permits at most 100 edits, 2 MiB content and a 10,000-entry tree; symlink/submodule edits are rejected. Staging refs remain after failure and need separately authorized reconciliation/collection. The adapter never deletes them without a ref guard.

Existing-branch replacement is `unsupported`: GitLab's documented commit API offers per-file `last_commit_id`, not branch-wide expected-head comparison. A read followed by a write cannot manufacture that guarantee. Publish a fresh companion ref until a native Git transport is independently qualified. [Commits API](https://docs.gitlab.com/api/commits/), [Branches API](https://docs.gitlab.com/api/branches/).

Protection combines matching protected-branch rules, effective operational permissions, project merge settings and approval rules. CODEOWNERS is required if any matching rule requires it; native effective `can_push` determines operational push authority. Hidden groups, overridable approvals, missing tier/fields and partial pagination block. MR approval state must be fully resolved and satisfy every applicable rule. Flat approval records retain actor identity without inventing a per-approval SHA absent from the API. [Protection semantics](https://docs.gitlab.com/user/project/repository/branches/protection_rules/).

Pipeline evidence binds the project and actual tested commit. Source pipelines require exact H; merged-result pipelines require a canonical two-parent commit containing exact H and B. Required statuses match the tested SHA and immutable publisher, with newer failures overriding earlier successes. Calculating, approval-syncing and unknown detailed merge states block.

## Native merge and train boundary

Direct native merge sends exact `sha=H`, disables automatic merge scheduling and honors the configured project method (`merge`, `rebase_merge`, `ff`) and squash policy. Automatic direct merge additionally requires observed native strict target behavior and live qualification. `automatic_rebase_enabled` must be explicitly false: GitLab can otherwise rebase at merge time without rerunning CI. An absent field is unknown. Immediate B reads do not replace native rejection/retesting when B advances. [Merge methods](https://docs.gitlab.com/user/project/merge_requests/methods/).

Train admission uses `POST /projects/:id/merge_trains/merge_requests/:iid`, exact H and `auto_merge=false`. It requires merged-results pipelines, visible enforced train policy and disabled skipping. Ordinary merge API auto-merge routing is not assumed to enter a train. A final canonical MR read rejects retargeting, source changes and state changes before the last execution guard. Successful direct merges require canonical outcome reconciliation. [Merge API](https://docs.gitlab.com/api/merge_requests/), [Train API](https://docs.gitlab.com/api/merge_trains/).

Train observation exposes its actual pipeline SHA as `TestedSHA`; it never substitutes H or invents a native target/predecessor binding. T11/T21 must associate complete candidate evidence, publish the execution-time policy gate and handle native cancellation/revocation races. Queue cancellation and deployment operations are not implemented in this slice. Native capabilities remain `unknown` until project qualification; deployment capability is `unsupported`.

## Validation and remaining qualification

Sixteen top-level fixture tests, including security subcases, pass under `go test -race ./internal/forge/gitlab`; `go vet ./internal/forge/gitlab` passes. Fixtures cover staged parent/tree mismatch, branch collisions, symlinks, revocation between writes, read-only inspector separation, hidden/partial approvals, wrong/newly failing status publishers, stale approvals, merged-result identity, automatic-rebase hazards, retargeting and dedicated train admission.

Before enabling writes, certify a dedicated GitLab.com project and each claimed Self-Managed version/tier: operational token scope/revocation, inherited protection and CODEOWNERS, staged-ref concurrency, source and target advancement at merge, automatic rebase settings, train skipping/enforcement, predecessor candidate changes, policy-gate revocation, cancellation races and restart/timeout reconciliation. Capture actual provider outcomes. No external credentials or live provider mutations were used.

Validation environment: `GOCACHE=/tmp/reforge-go-build GOMODCACHE=/tmp/reforge-go-mod GOPATH=/tmp/reforge-go`.
