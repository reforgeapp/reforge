# Maintenance, merge and deployment policies

Updated: 2026-09-20. Selected v1 design for Reforge; acceptance requirements, not implemented guarantees. Applies equally to hosted and self-hosted editions. The Go policy evaluator is deterministic and independent of model output.

## Resolution and authority

Resolve policy using the repository's organisation, all explicitly bound teams, repository binding and deployment hard limits. Labels do not grant authority. Store the binding versions and resolved canonical hash in every decision.

| Setting | Resolution rule |
| --- | --- |
| Explicit deny, forbidden paths, required validation/reviews | Union across deployment, organisation, bound teams and repository; any applicable deny wins. |
| Allowed recipes, models, routes, merge methods, environments, workflows | Intersection of explicit ancestor and descendant allowlists. Missing child value inherits; explicit empty list permits nothing. |
| Budget, concurrency, attempts, patch size, open changes | Most restrictive applicable ceiling; independent scope budgets must all have capacity. |
| Required gates and approval counts | All applicable gates; highest required count for the same rule, while distinct approval rules remain distinct. |
| Ordinary defaults such as selected model profile or branch prefix | Repository overrides one explicitly designated primary team's defaults, then organisation defaults; selected value must satisfy every ancestor constraint. |
| Multiple teams | All team constraints apply; only the primary team supplies defaults. Missing/ambiguous primary team blocks actions needing an unresolved default. |
| Missing or invalid policy | Read-only inventory and explanations remain available; no automated repair, publish, merge or deployment. |

Scope administrators can tighten policy within their authority. An organisation policy administrator can explicitly change organisation constraints; a repository administrator cannot escape them through a local override. Changes require version checks, simulation, immutable `PolicyVersion`, actor and reason. Raising ceilings or removing requirements is an explicit policy change, not a run-specific “ignore” button.

Owner sets organisation constraints. Admin manages assigned policy scopes within those constraints. Maintainer queues permitted work. Reviewer grants only authorised application approvals. Neither role nor an application approval grants missing native forge/deployment authority. Workers and models have no merge, deployment or policy-edit authority.

Repository PRs, instructions, bot comments and model output are untrusted policy inputs. Proposed configuration changes never become active merely because they exist on a task branch or merge into a repository. A policy administrator must import, simulate and activate the immutable policy version. The change proposing weaker policy cannot authorise itself. Read governing repository configuration from the trusted activated revision, not the candidate being evaluated.

Every policy activation emits `policy.changed`; affected queued/in-flight decisions become stale. Re-evaluate before the next action. Record both the task's starting policy and the currently effective policy; an older task cannot retain grandfathered merge/deployment authority.

## Decisions and evidence

An action evaluation returns `allow`, `deny` or `unknown`, with matched rule IDs, blockers, required actions and evidence references. Only `allow` can dispatch a mutation. Denial is a known violated requirement; unknown is missing, stale, unsupported or unreadable evidence. Unknown never becomes allow through timeout or a missing API field.

Provider capability state remains `supported/unsupported/unknown` as defined in `contracts.md`. A required unsupported capability blocks that action. Do not confuse supported capability with satisfied gate: a supported approval API can report an unmet review.

| Evidence | Binding and invalidation |
| --- | --- |
| Candidate | Immutable source repository ID, source ref and head `H`; candidate tree/diff digest. |
| Target | Immutable target repository ID, target ref and current target tip `B`; never substitute a diff merge-base or an old target snapshot. |
| Validation | Exact baseline, candidate and target SHAs, tested commit `T`, command/version, environment fingerprint, result and logs. |
| Policy | Effective policy hash, parent/binding versions, provider-rules hash, capability version and observation time. |
| Reviews | Native identities, rule satisfaction, review revision and dismissal state; separate app approvals bound to H/B/policy. |
| Queue/train | Native entry ID, head, tested combined candidate SHA, target and predecessor set where observable. |
| Deployment | Merge/source SHA, artifact digest, environment, approved workflow/config revision and deployment run identity. |

`GateEvaluation` is an auditable snapshot, not a durable permission token. Re-fetch canonical provider state and re-evaluate immediately before mutation; enforce configured freshness bounds. Head, target, policy, required checks, approvals or relevant queue candidate changes invalidate the affected evaluation. Provider rules remain authoritative even when the app's stricter policy passes.

## Repair and validation

Default flow: reproduce baseline → bounded patch → candidate verification → publication → native checks. Run baseline and candidate using the same approved validation recipe and comparable isolated environments. Retain failures, skipped commands, missing dependencies, flaky results and resource limits explicitly.

- Baseline for a dependency repair is the exact failing bot head; target-branch validation is recorded separately. For a regression repair, reproduce the identified defect at the recorded unfixed revision.
- A new regression test should fail on unfixed code and pass on the candidate where feasible. If reproduction is unavailable, label the result unproven and require review; it cannot enter the default autonomous merge lane.
- A failed required baseline check unrelated to the task is not silently waived. Classify it with evidence; the candidate must satisfy all required merge gates. A human may narrow a recipe through authorised policy change, never by rewriting a failure as success.
- Disabling tests, excluding failing cases, reducing assertions/coverage thresholds, replacing a command with a no-op, or weakening scanner/CI rules cannot satisfy verification. Test changes must preserve intent and pass the review policy for that recipe.
- Retries are bounded and reported together; one passing retry does not erase previous failures. Flakiness requires the explicit validation policy, not the model's assurance.
- Changed dependencies, lockfiles, test tools, fixtures or workflow configuration can invalidate comparability. Re-run the relevant baseline/candidate checks and require review for material validation changes.
- New candidate commits invalidate prior candidate evidence. “Validated locally”, “native CI passed”, “eligible to merge” and “deployed healthy” are separate facts.

Default merge lane permits bounded low-risk repairs only. Workflow/authentication files, production infrastructure, broad API migrations, major runtime/framework upgrades, destructive data changes and validation-policy edits require explicit higher-risk recipes and review. Patch and resource ceilings apply before publication.

## Dependency bot coordination

Discover Renovate/Dependabot configuration and open work before planning dependency changes. Preserve the existing bot as dependency version resolver. Record immutable actor/installation identity and change provenance; branch prefixes, labels or display names alone cannot prove ownership.

Deduplicate by connection/repository ID, target branch, ecosystem, manifest set, package/group and requested version/range/digest. Recognise partial group overlap and superseding updates. Reconcile open changes again before publishing; ambiguous ownership blocks a second version-bump proposal.

Default `bot_branch_writes=false`: publish an app-owned companion repair linked to the original bot change. The companion contains compatibility work, not a competing dependency upgrade. Record the combined tested candidate and explicit merge dependency/order. If compatibility work can safely merge first, validate it independently and revalidate the bot update afterward. If neither change is independently valid, require a reviewed handoff/consolidation plan; do not automatically merge two individually invalid branches or pretend cross-PR atomicity.

In-place bot-branch repair is repository opt-in under organisation policy. Require trusted ownership, exact observed H, no unreviewed human edits, a branch lease and a certified exact old-ref guard. Append a descendant repair commit; never amend bot commits or rewrite someone else's work. Record that Reforge has taken responsibility for branch maintenance. A rejected guard stops the attempt and refreshes state.

Renovate and Dependabot normally stop automatic rebasing after added commits; destructive regenerate/rebase controls can overwrite edits. Never apply those controls after a repair without explicit handoff authority. Do not include Dependabot skip markers that permit overwriting repair commits.

A bot force-push, refreshed version/group, branch replacement or human commit invalidates all dependent combined-candidate tests, approvals and merge decisions. Companion changes become stale until rebuilt against the new bot head. Preserve unpublished repair artifacts; do not automatically replay them onto an unrelated update.

One merge authority controls each policy scope. If Reforge controls merge, existing bot automerge must be disabled through reviewed configuration or constrained by a certified provider-enforced Reforge gate. Discovery does not silently reconfigure the bot. Until cooperation is established, observe and propose repairs without claiming control of its merges.

## Merge policy and provider differences

All merge requests require an exact head guard, current target snapshot, satisfied native rules and current application policy. Require open/non-draft state, allowed method, trusted required checks, applicable reviews/CODEOWNERS, resolved native blockers and the correct operational actor. No bypass identity, force-merge or direct protected-target push is an automation fallback.

| Provider | Execution policy |
| --- | --- |
| GitHub | Read both relevant branch protection and rulesets; preserve check publisher identity. Honour required merge queue and `merge_group` evidence. Send exact source-head guard on the certified merge/queue API. Cloud and Enterprise Server capabilities are certified separately. |
| GitLab | Read detailed merge status, applicable approval rules, pipeline evidence and protected-branch settings. A successful pipeline or approver list alone is insufficient. Use merge trains when configured; send exact source `sha` for admission/merge. Transitional calculation states remain unknown. |
| Gitea | Pin certified server versions. Verify effective protection, stale-review/outdated-branch behavior and exact-head rejection; always disable force merge. CODEOWNERS uses provider-specific syntax and version-dependent enforcement. Missing required CODEOWNERS/check enforcement blocks autonomous merge. Scheduled auto-merge is not evidence of a speculative queue. |

Native queues/trains test the combined candidate against target/predecessors. Without one, permit direct automation only when the certified provider atomically enforces required up-to-date/fast-forward behavior and checks for the correct candidate. Reforge's per-target lease serializes Reforge, not people or other bots.

The common APIs do not provide a universal atomic comparison of both H and B. An immediate preflight target read alone cannot eliminate the race. If provider behavior cannot establish rejection/retesting when B advances, show `unknown: target freshness not enforceable` and stop automatic merge. Do not claim that passing H plus a local mutex solves it.

Native queue admission/auto-merge hands an action to another system. On policy change or pause, immediately invalidate local decisions, attempt supported cancellation/dequeue, and reconcile the provider outcome. Execution-time Reforge gate enforcement is required when policy promises a final app-controlled decision; even then document the adapter's actual revocation boundary. A cancellation can lose a race with an already executing merge. Never promise an absolute kill switch or mark cancellation successful without observation.

## Independent lifecycles

The following are proposed enum sets for `contracts.md` records; record transitions transactionally with audit/outbox events. Never set one aggregate's state by assuming another aggregate's outcome.

| Record | Normal transitions | Exceptional transitions |
| --- | --- | --- |
| Task | `queued → reproducing → planning → repairing → validating → publishing → completed` | Active stage → `blocked`, `failed` or `cancelling`; cancellation confirmed → `cancelled`. Uncertain publication → `reconciling`. Resume/retry creates an Attempt and returns to the earliest safe stage. |
| Change | `draft → open → evaluating → eligible → merge_requested → queued → merged` | Direct merge can skip queued. Gate drift → `blocked`; new head → `evaluating`; uncertain mutation → `reconciling`; observed close → `closed`. Native rejection never becomes merged. |
| Deployment | `planned → awaiting_gates → eligible → requested → running → verifying → healthy` | Missing gate → `blocked`; uncertain invocation → `reconciling`; failure → `failed`; completed pipeline without configured health evidence → `completed_unverified`; authorised recovery → `recovery_requested → recovering → recovered` or `recovery_failed`. Confirmed cancellation → `cancelled`. |

Task `completed` means its defined repair/verification/publication work completed, not that the change merged. Change `merged` requires canonical merge evidence and merge SHA, not an accepted HTTP response. Deployment `healthy` requires attributed health evidence, not a merge or successful trigger. `recovered` identifies the restored revision/artifact; it does not relabel the failed deployment healthy.

After an external timeout, reconcile using operation ID, provider ID and immutable revisions before retry. A stale lease/fencing token cannot publish or request further actions. Recovery retains the failed attempt's history and evidence.

## Campaigns and bounded progression

Campaign start pins repository membership, recipe version, policy bindings, model route, canary set, stage sizes/concurrency, success criteria, observation window and failure/rollback thresholds. Selection changes require a new preview/version; a saved filter does not silently add repositories to a running campaign.

Progress `planned → canary → observing → expanding → completed`; any stop condition leads to `paused` or `failed`. Pick representative canaries across stacks, forge versions and validation paths. Advance only after the configured sample completes its relevant lifecycle: deployment campaigns require healthy deployments, repair-only campaigns require their stated validation/review outcome.

Canary failure, regression, recovery, policy bypass, unattributed deployment, exhausted budget or unavailable required evidence stops new stage dispatch. Failed/blocked/skipped repositories stay visible in the denominator. No advancement because a timeout elapsed. Resume requires an authorised review, refreshed eligibility and explicit stage decision; policy or recipe changes require renewed canaries where affected.

Campaign pause propagates to pending member actions; external queue/deployment cancellation uses the same qualified semantics as repository pause. No cross-repository atomic release or automatic all-or-nothing rollback is promised.

## Deployment and recovery

v1 orchestrates allowlisted existing CI/CD workflows and GitOps changes. Policy binds workflow ID/version, environment, actor, permitted source refs, artifact provenance, health checks, deadlines, native approvals and recovery procedure. A model cannot select arbitrary workflow inputs, credentials or production commands.

Bind each promotion to exact merged source SHA and immutable artifact digest. If the pipeline builds the artifact after invocation, wait for authenticated build provenance linking that digest to the pinned SHA before approving promotion. Native run reruns/new artifacts are distinct observations; an approval for a previous artifact cannot silently apply to a replacement.

Native protected-environment/reviewer gates remain mandatory; Reforge approval never substitutes for them. GitHub/GitLab gate availability depends on installation/plan. For Gitea or an external deployer, require a certified enforcement adapter or use observe-only deployment mode. Deployment-status reporting alone does not enforce an approval gate.

Required v1 portable delivery path: a protected GitOps repository on any supported forge. Reforge proposes a deterministic, allowlisted manifest field change to an immutable artifact digest, through that repository's ordinary PR/MR and approval rules. Existing reconcilers perform deployment; Reforge consumes authenticated read-only health/provenance evidence. This provides a Gitea deployment path without inventing native environment gates. When an organisation requires an additional external approval gate, its enforcement must also be certified before automated promotion. All source and delivery repositories must be authorised in the same organisation; no implied cross-tenant access.

Serialize deployment intents per environment and reconcile already-running native jobs. Health evidence must identify the actual deployed artifact/revision and relevant environment, remain fresh for the observation window and include configured smoke/error criteria. Missing attribution, partial rollout or conflicting deployer state is unknown, not healthy.

Recovery requires a preauthorised existing pipeline action or GitOps revert procedure, known-good immutable target, applicable native approvals and separate execution evidence. Automatic recovery is allowed only within that previously authorised envelope. Otherwise stop progression and request authorised recovery; never invent a production command or assume database reversibility.

For Kubernetes, every mutation is represented in GitOps. A revert is a reviewed/policy-authorised Git change consumed by the existing reconciler, never a direct API patch, rollout restart or cluster-admin operation. Read-only cluster health evidence may inform verification. Recovery can fail and cannot undo every external effect.

## Pause, cancellation and external outcomes

Pause scopes: organisation, repository, recipe, campaign, model connection and runner pool. Any applicable pause blocks new affected dispatch and the next Reforge-controlled mutation. A model-connection/pool pause blocks dependent work; it does not implicitly cancel unrelated already-published changes unless the pause explicitly includes their merge/deployment actions.

On pause, persist its version, revoke pending local dispatch, stop affected runners and request cancellation of supported native queued work. Keep observation/reconciliation active. Show `cancellation requested`, `confirmed stopped`, `completed before cancellation` or `outcome unknown` accurately. A separate authorised recovery may be allowed under pause; it must not silently resume normal automation.

Pause cannot undo a published PR, completed merge, started irreversible job, deployment side effect or already incurred model charge. External systems can cross their commit point before cancellation arrives. Show outstanding provider operations and their last observation; do not claim all activity stopped while any outcome is uncertain. Resume never reuses old gate snapshots.

## Acceptance matrix

| Failure or race | Required result |
| --- | --- |
| Repository policy permits an organisation-denied path/action | Deny with ancestor rule; candidate policy cannot activate itself. |
| Conflicting team constraints/defaults | Intersect constraints; unresolved default blocks action with both bindings shown. |
| Required provider API unavailable, forbidden, paginated incompletely or returns unknown enum | Unknown/block; no interpretation as absent protection. |
| Head or target changes after validation/preflight | Invalidate; exact-head server guard and certified target enforcement reject or retest. Otherwise no autonomous merge. |
| Same-named green check has wrong publisher/SHA; approvals stale | Block; retain native rule/evidence identity. |
| Two green changes fail when combined | Queue/train or serial fresh candidate detects failure; no reliance on app lease alone. |
| Gitea release lacks proven required enforcement | Repair/PR creation can continue; affected merge/deployment capability remains blocked. |
| Bot and worker push concurrently; bot force-pushes after companion validation | Exact ref guard preserves commits; companion/evidence invalidated; no automatic overwrite. |
| Bot group overlaps an existing repair/update | Adopt/reconcile or block; no second independent upgrade proposal. |
| Candidate deletes a failing test, weakens a gate or only passes selected retries | Reject evidence; full results remain visible. |
| Worker dies or provider times out after publication/merge/trigger | Reconcile original operation before retry; no duplicate intent invocation. |
| Pause/revocation races native queue or deployment execution | Stop local boundaries, request cancellation, report observed result; never assert impossible rollback/absolute stop. |
| Task completed but native CI fails | Task history retained; Change blocked; Deployment not created as successful. |
| Canary fails, rolls back or has missing required evidence | No stage expansion; preserve sample denominator and authorised resume decision. |
| Deployment artifact/SHA differs, required approval absent or health unattributed | Block promotion/healthy state; do not substitute tracking status for enforcement. |
| Recovery route not preauthorised or known-good artifact unavailable | Pause campaign/deployment progression; require authorised action, no direct Kubernetes mutation. |
| Connection revoked, tenant mismatch or stale fencing token | No new mutation; reconcile previously dispatched operation under appropriate authority. |

Domain records/interfaces: [contracts](contracts.md). Execution isolation and leases: [architecture](architecture.md).
