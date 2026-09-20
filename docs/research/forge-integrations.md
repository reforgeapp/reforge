# Forge integration research

Research date: 2026-09-20. Scope: Go backend, hosted multi-tenant and self-hosted installations, large repository portfolios. Documentation research only; no live integrations, merge experiments, or deployments were performed. “Documented” below describes official documentation. All contracts, defaults, acceptance tests, and fallback behavior are proposed design requirements, not verified implementation.

## Capability matrix

| Capability | GitHub Cloud / Enterprise Cloud / Enterprise Server | GitLab.com / Self-Managed | Gitea |
| --- | --- | --- | --- |
| Create changes | REST creates PRs; merge accepts a head `sha`; branch update accepts `expected_head_sha`. [Pulls API](https://docs.github.com/en/rest/pulls/pulls) | REST creates MRs and merges with source-head `sha`. Use `detailed_merge_status`; status calculation can be asynchronous. [MR API](https://docs.gitlab.com/api/merge_requests/) | REST creates PRs. Merge schema includes `head_commit_id`, `force_merge`, and `merge_when_checks_succeed`. Presence in schema does not prove race safety; certify each supported release. [Create](https://docs.gitea.com/api/1.26/operations/repo-create-pull-request/), [merge](https://docs.gitea.com/api/1.26/operations/repo-merge-pull-request/) |
| Rules and checks | Branch protection and rulesets are separate inputs; rules can require reviews, checks, queue, signatures and other conditions. Preserve expected check-app identity. [Rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets), [rules API](https://docs.github.com/en/rest/repos/rules) | Protected branches, project settings, approval rules and merge checks are separate inputs; do not reduce eligibility to successful pipeline or approval count. [Approval API](https://docs.gitlab.com/api/merge_request_approvals/), [MR API](https://docs.gitlab.com/api/merge_requests/) | Protection supports review restrictions, stale approvals, status patterns, outdated-branch blocking, file restrictions and admin enforcement. First matching branch rule applies; do not combine rules using GitHub semantics. [Protection](https://docs.gitea.com/usage/access-control/protected-branches/) |
| CODEOWNERS | Required code-owner approval is an enforceable PR rule when configured. [Rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets) | Code Owner approval requires appropriate tier and protected-branch configuration. [Code Owners](https://docs.gitlab.com/user/project/codeowners/) | Syntax uses Go regex and different file precedence. `next` docs describe per-applicable-rule code-owner approval, while stable overview does not establish that support on every release. Discover and test version; otherwise report unsupported/unknown. [Stable](https://docs.gitea.com/usage/repository/code-owners/), [next](https://docs.gitea.com/next/usage/repository/code-owners/) |
| Queue / train | Native merge queue tests combined changes. CI must handle `merge_group` / queue refs. Cloud availability depends on repository ownership, visibility and plan; Server needs version-specific certification. [Queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue) | Native merge trains test earlier train entries together; Premium/Ultimate. Merged-results pipeline alone does not serialize concurrent merges. [Trains](https://docs.gitlab.com/ci/pipelines/merge_trains/), [train API](https://docs.gitlab.com/api/merge_trains/) | Scheduled auto-merge exists in API. No native speculative queue equivalent was established by the reviewed docs. Proposed fallback: serialize app decisions and require provider-enforced up-to-date branches plus fresh tests. This is not a native queue. [Merge API](https://docs.gitea.com/api/1.26/operations/repo-merge-pull-request/) |
| Deployment gates | Environment rules gate jobs and secret access; reviewer/wait features depend on plan and visibility. [Environments](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments) | Protected environments and deployment approvals require Premium/Ultimate. [Protected environments](https://docs.gitlab.com/ci/environments/protected_environments/) | GitHub Actions compatibility does not establish equivalent deployment protections. External deployment adapter must prove its own gates; unavailable capability blocks automated promotion. [Actions comparison](https://docs.gitea.com/usage/actions/comparison/) |

Release policy recommendation: support all three forge families for discovery, repair branches and PR/MR creation in v1. Enable merge/deployment automation per certified capability, not provider name. Record instance version, edition, license-derived capability, API version, authentication type and last successful probe. Pin a tested self-hosted version range before GA; current cloud documentation must not silently become a Server compatibility promise.

## Authentication and permissions

| Provider | Proposed onboarding and credential boundary | Minimum operational scope and limitations |
| --- | --- | --- |
| GitHub | Prefer repository-selected GitHub App installations; separate instance registration for Enterprise Server. Tenant-scoped secret reference, installation ID and endpoint identity. Installation tokens expire after one hour and can be narrowed to allowed repositories/permissions. [Token documentation](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app) | Contents read for inventory, write for branch writes/merge; Pull requests write for PR creation/update. Checks/statuses read for evidence; only the app's own policy check needs write authority. Branch-protection inspection requires Administration read where documented; no Administration write or bypass privilege in the operational identity. Add Actions/Deployments permissions only for selected features and endpoint requirements. [Pulls](https://docs.github.com/en/rest/pulls/pulls), [protection API](https://docs.github.com/en/rest/branches/branch-protection#get-branch-protection) |
| GitLab | Prefer dedicated project or group bot identity; project token limits scope, group token reduces onboarding overhead at larger blast radius. OAuth user credentials are an explicit alternative, not permanent dependence on an employee account. Project token availability varies by offering/tier. [Project tokens](https://docs.gitlab.com/user/project/settings/project_access_tokens/) | `read_api` for observation; `api` for MR mutations, subject to bot role and protected-branch permissions. `write_repository` permits Git-over-HTTP writes but does not authenticate API writes. Grant webhook administration separately during setup if needed. [Scopes](https://docs.gitlab.com/security/tokens/access_token_scopes/), [webhook prerequisites](https://docs.gitlab.com/user/project/integrations/webhooks/) |
| Gitea | Dedicated non-admin service user and scoped token; restrict its actual repository memberships. OAuth is an optional onboarding path after release-specific certification. Never collect a user's password to mint a token. | Begin with repository read; add `write:repository` and only operation-required `write:issue` for change/label management, plus read organization/user for selected discovery features. Scope names are `read:`/`write:` categories, with write implying read. Check endpoint requirements on the connected instance. Never `all`, admin, or sudo by default. [API usage](https://docs.gitea.com/1.26/development/api-usage/) |

Design: separate discovery, repair, merge and deployment authorization in the application even when the provider token cannot express that separation. Keep credentials in a tenant-scoped encrypted store; workers receive short-lived task credentials where supported. Removing an installation/repository immediately disables scheduled mutations. A forbidden rules endpoint is “policy unreadable”, never “unprotected”. Explain the missing permission without escalating the credential automatically.

Self-hosted instances need explicit endpoint and CA configuration. Hosted service access to private instances should use an outbound tenant runner/connector or an explicitly configured reachable endpoint. Instance URLs require host allowlisting, DNS/IP validation and redirect restrictions; repository URLs from webhook payloads cannot select arbitrary destinations. Do not forward tokens across origins. Mirror the same connector contract in the self-hosted product.

## Merge eligibility and exact revisions

Design invariant: a successful worker test is evidence for a particular `(source repository, H, target repository, target ref, B, T, policy revision)` where `H` is PR/MR head, `B` is the current target tip, and `T` is the exact tested head or merge candidate. A diff merge-base is not `B`. GitLab explicitly distinguishes diff merge-base, head and target-at-diff-creation SHAs. [MR SHA definitions](https://docs.gitlab.com/api/merge_requests/#shas-in-the-api-response)

Before requesting merge:

1. Re-fetch change, both refs, effective provider policy and app policy; require open, non-draft, permitted actor/method and no conflicts. Invalidate prior authorization when head, base, policy or relevant review/check state changes.
2. Fetch every page of required checks, approvals and unresolved blocking discussions. Retain provider-native state and source identity. Missing, pending, stale, ambiguous, cancelled or unknown required evidence blocks merge. A successful unrelated pipeline or same-named check from another publisher does not satisfy the requirement.
3. Verify actual required approval rules, including CODEOWNERS and stale-review behavior. GitLab `approved_by` alone includes approvers who might satisfy no rule; use applicable `/approval_state` rules. Do not use this app, Renovate helper identities, or delegated workers to manufacture human approval. [Approval semantics](https://docs.gitlab.com/api/merge_request_approvals/)
4. Choose a certified merge path. Queue/train mode validates the generated candidate against its real base and predecessor set. Direct mode requires provider-enforced up-to-date/fast-forward semantics and checks on the resulting updated head. A local app mutex does not serialize human or other-bot merges.
5. Submit with exact head guard; re-observe outcome before marking merged. An accepted asynchronous request is `merge_requested`/`queued`, not `merged`. A timeout is an uncertain result; reconcile before retrying.

| Provider request | Revision guard | Specific constraints |
| --- | --- | --- |
| GitHub `PUT /repos/{owner}/{repo}/pulls/{number}/merge` | Send `sha=H`; mismatch gives conflict. | No common expected-target-SHA parameter is documented. Use native queue when required. Current REST docs also describe async merge/queue actions; capability-gate those newer endpoints instead of assuming Enterprise Server parity. [Pulls API](https://docs.github.com/en/rest/pulls/pulls) |
| GitLab `PUT /projects/:id/merge_requests/:iid/merge`, or `POST /projects/:id/merge_trains/merge_requests/:iid` | Send `sha=H` on both direct merge and train admission. | `detailed_merge_status` transitional states require another read. Train API distinguishes immediate admission from scheduled admission; use modern `auto_merge` only where supported. [MR API](https://docs.gitlab.com/api/merge_requests/), [train API](https://docs.gitlab.com/api/merge_trains/) |
| Gitea `POST /repos/{owner}/{repo}/pulls/{index}/merge` | Send `head_commit_id=H`; certify mismatch rejection in integration tests. | Explicit `force_merge=false`; no manually-merged shortcut. Set a permitted merge method. Do not infer an expected-base guard from `merge_commit_id`. [Merge schema](https://docs.gitea.com/api/1.26/operations/repo-merge-pull-request/) |

No reviewed common endpoint atomically compares both H and B. A preflight read alone cannot close the target-advance race. If the adapter cannot demonstrate provider enforcement that rejects stale direct merges or retests queue/train candidates, automated merge stays disabled. A repository can remain fully usable for diagnosis and proposed repairs. Policy requiring an app-controlled final decision at execution time also needs a provider-enforced app check/gate on the candidate; merely enabling provider auto-merge and later cancelling it has a race.

## Events and portfolio reconciliation

Documented provider differences:

- GitHub delivery identifiers survive redelivery; use the delivery ID to deduplicate. Review event/action and redeliver missed events where authorized. [Webhook practices](https://docs.github.com/en/enterprise-server%403.20/webhooks/using-webhooks/best-practices-for-using-webhooks)
- GitLab 19.0 introduced webhook signing tokens and 19.1 removed the feature flag. Older installations use `X-Gitlab-Token`. New integrations should use signed payload verification when supported. Webhooks can be automatically disabled after repeated failures. [Webhooks](https://docs.gitlab.com/user/project/integrations/webhooks/)
- Gitea signs the raw body with HMAC-SHA256; `X-Gitea-Signature` has no `sha256=` prefix. Empty configured-secret signatures must be rejected. [Webhooks](https://docs.gitea.com/usage/repository/webhooks)

Design: webhook handlers verify before parsing, persist a bounded event then acknowledge. Authenticate the connection before mapping provider repository IDs to tenant-owned repositories; never trust a payload tenant ID. Events enqueue reconciliation, not direct merge. Subscribe to push, change lifecycle, reviews, checks/pipelines, deployment and installation/access changes as supported. Unknown event types are retained for diagnosis without causing mutations.

Poll fallback scans active work frequently, the repository portfolio incrementally, and full inventory on a slower cadence. Use pagination/checkpoints, overlapping update windows, conditional requests when supported, provider Retry-After/rate-limit headers, jitter and per-tenant fairness. A completed sync watermark is advanced only after all pages succeed. Track event lag, polling lag, inaccessible repositories and credential expiry. Lost events and out-of-order events are repaired through canonical API reads.

## Renovate and Dependabot cooperation

Documented: Renovate supports the target platform families, so reuse it for dependency discovery, grouping and upgrade generation rather than rebuilding package managers. Renovate identifies existing updates partly through branch/title conventions; app deduplication must be stronger than title matching. [Platforms](https://docs.renovatebot.com/modules/platform/), [PR behavior](https://docs.renovatebot.com/key-concepts/pull-requests/)

Design defaults:

1. Maintain a repository dependency ownership map by ecosystem, manifest path, target branch and package/group. Detect installed/configured bots and existing open changes before generating anything. Fingerprint intended changes using dependency identity, current/target constraint or digest, manifest set and group membership. Partial overlap also blocks duplicate creation pending reconciliation.
2. Let the existing bot own version selection and initial PR/MR. This app owns diagnosis, bounded compatibility repairs, verification and policy coordination. Record trusted bot actor/installation IDs; labels, usernames and branch prefixes alone are insufficient ownership proof. Dependabot presence on GitLab/Gitea is not assumed; recognize explicitly configured external deployments only.
3. Exactly one merge authority per repository/policy scope. When this app controls merging, require bot automerge to be disabled or gated by a provider-enforced app policy check. Propose bot configuration changes through reviewed PRs/MRs. Never silently turn off an existing bot or weaken branch protection.
4. Prefer requesting bot regeneration/rebase before editing an untouched bot branch. Never request destructive regeneration once another author or repair worker has added commits. Outbound comment commands are an optional separately authorized feature, not necessary for the core integration.

Renovate stops branch updates after an additional commit; amended bot commits can be overwritten. Its rebase label can regenerate even a modified branch. Therefore an authorized in-place repair must append a new commit, transfer branch maintenance responsibility visibly in application state, and never amend or automatically apply the rebase label afterward. [Updating and rebasing](https://docs.renovatebot.com/updating-rebasing/)

Dependabot also normally stops rebasing after additional commits, while documented skip markers explicitly permit overwriting those commits. Repairs must avoid those markers and record responsibility for keeping the branch current. [Dependabot PR management](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/manage-your-dependency-security/manage-dependabot-prs?learn=dependabot_alerts)

Design for safe writes: record observed H, claim a durable per-change lease, fetch H, generate an append-only repair, and compare-and-swap only the approved source ref. Re-read immediately before submission; a plain non-fast-forward rejection does not protect against every branch rewind, so certify an exact old-ref guard or equivalent explicit Git lease with a descendant commit. Branch update capability unavailable, fork permissions missing, bot activity detected, or human edits present: create a linked repair proposal on an app-owned branch or require human handoff. Do not close the original PR automatically. Do not run both proposals toward merge independently. All new heads invalidate checks/approvals according to policy.

Renovate platform automerge depends on host policies actually requiring tests; it does not inherently guarantee a fresh base. Configure PR-based coordination and preserve queue/train rules; never adopt a branch-push bypass suggestion. [Platform automerge](https://docs.renovatebot.com/configuration-options/#platformautomerge), [automerge behavior](https://docs.renovatebot.com/key-concepts/automerge/)

## Deployment contract

Design: merging, building, approving promotion, requesting deployment and observing healthy rollout are separate states. Bind promotion approval to immutable artifact digest, source/merge SHA, environment, config revision, policy revision and expiry. A new artifact/environment invalidates approval. Serialize promotions per environment and verify deployment completion from the actual deployer.

Use GitHub environment or GitLab protected-environment gates when they truly control execution. Merely publishing deployment status is not enforcement: GitLab explicitly excludes protected environments/approvals from external deployment tracking. External deployers need their own authenticated policy-enforcement contract and observable outcome. [External deployment limits](https://docs.gitlab.com/ci/environments/external_deployment_tools/)

For Kubernetes, generate deployment/configuration changes in a GitOps repository and follow the same PR/MR protections. Never mutate the cluster directly. Read-only rollout observations can satisfy health evidence; they cannot approve promotion. Unsupported native gates, stale health data, missing artifact identity or unavailable approval verification block automated promotion. Do not auto-approve a human deployment gate using the merge bot credential.

## Proposed Go integration boundary

Keep forge adapters separate from policy and deployment adapters; no lowest-common-denominator `CanMerge bool`.

| Operation / model | Required contract |
| --- | --- |
| `DiscoverCapabilities` | Instance identity/version, endpoint/API version, supported/unsupported/unknown per capability, permission status, evidence timestamp and reason. |
| `ListRepositories`, `ListChanges`, `GetChangeSnapshot` | Tenant + connection + immutable repository ID; cursor pagination; head repo/ref/SHA, current target tip, draft/conflict state and original provider payload references. |
| `ReadEffectivePolicy`, `ReadEvidence` | Native rule identifiers and revisions, freshness, checks with publisher and tested SHA, approval rule satisfaction, queue/train candidate identity, explicit unknown fields. |
| `CreateChange` | Intent key and deterministic owned branch; discover existing matching changes before retry; return original provider ID and URL. |
| `AppendRepair` | Expected source H, owned/authorized branch, new descendant commit, exact ref guard, durable ownership transition. No implicit force rewrite. |
| `RequestMerge`, `CancelMerge`, `ObserveMerge` | Immutable decision record; expected H/B/T and policy hash; allowed method/path; idempotent intent key; return requested/queued/merged/rejected/unknown with canonical provider outcome. |
| `VerifyWebhook`, `Reconcile` | Provider-native authentication; tenant-scoped event ID; durable inbox; canonical reads; replay-safe processing. |
| `DeploymentAdapter` | Observe gates, request permitted action/GitOps proposal, observe immutable artifact and health. A tracking-only adapter cannot claim gate enforcement. |

Persist workflow transitions and an outbox in the same transaction. Use `(tenant, connection, repository ID, change ID)` for locks and deduplication; names alone are mutable. Include tenant identity in token caches, API caches, workspaces and evidence storage. Retain redacted request IDs and decision inputs for audit. Classify failures as retryable, authorization-required, unsupported, stale-precondition, policy-denied, or uncertain-outcome.

## High-value acceptance tests and unresolved certification

These tests are proposed release gates; none were executed during research.

| Test | Required result |
| --- | --- |
| Head changes after tests, between preflight and merge, or during repair | Server rejects stale head/ref; no lost commit and no old approval reused. |
| Base advances after checks or between last read and merge | Queue/train regenerates and retests, or provider rejects stale direct merge. If neither can be proven, automation remains disabled. |
| Two PRs pass separately but conflict semantically together | Later candidate is tested with earlier candidate/base; app mutex alone cannot pass this test. |
| Native required review/CODEOWNERS missing, stale or dismissed | No merge; validate actual provider matching semantics, including Gitea regex and GitLab rule eligibility. |
| Checks absent, wrong SHA, same name/wrong publisher, paginated away, cancelled, or unknown enum | No merge; surface actionable reason. Test queue/merged-result SHAs distinct from PR head. |
| Bot with bypass/admin rights accidentally connected | Block automated merge until a non-bypass operational identity or certified enforcement is established; never call force-merge. |
| Timeout after provider created PR or accepted merge | Reconciliation finds original action; no duplicate PR or repeat mutation. |
| Bot and worker update branch concurrently; human commit appears | Exact ref guard prevents overwrite; stop and preserve all commits. No automatic destructive rebase. |
| Duplicate bot proposal/group overlap or both bots installed | Existing update is adopted or conflict surfaced; no duplicate upgrade proposal. |
| Forged, replayed, delayed, dropped, cross-tenant webhook; token revoked mid-job | Reject unauthorized events; poll restores state; no mutation in another tenant. |
| Policy revoked while native auto-merge is pending | Provider-enforced app gate blocks execution; cancellation alone is not accepted as proof. |
| Deployment reports success for wrong SHA/digest or approval is stale | Promotion/healthy state remains blocked; tracked status cannot substitute for execution enforcement. |
| Old Server/GitLab/Gitea versions or license changes | Capability becomes unsupported/unknown; creation still works where certified, unsafe automation stays off. |

Outstanding before GA: choose and test supported self-hosted version ranges; certify Gitea exact-head/ref guards and CODEOWNERS enforcement by release; establish native queue absence/presence on each supported Gitea edition; validate each deployment adapter's enforcement; verify API pagination, permissions and rate limiting using least-privilege accounts. These are implementation acceptance gates, not reasons to omit a required forge from v1.

Confidence: 92% in documented distinctions and proposed safety constraints; provider behavior still requires version-specific integration certification.
