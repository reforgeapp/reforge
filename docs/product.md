# Product specification

Updated: 2026-09-20. Proposed product decisions except where marked agreed in `../PLAN.md`.

## Product

An OSS application for ongoing maintenance across repository portfolios. Teams connect forges and model accounts, choose policies, and supervise work from discovery through verified change and deployment in one GUI.

The application supplies scheduling, evidence, policy decisions and continuity. Model execution is replaceable. The application is not an IDE, a generic agent chat workspace, or a new package-update resolver.

Positioning hypothesis: the complete Apache-2.0 maintenance workflow for teams across GitHub, GitLab and Gitea, with model choice, dependency-bot cooperation and policy-controlled delivery. OpenHands is a close existing GUI/BYO/automation competitor; validate Reforge's workflow advantage during the private alpha. The [competition review](research/competition.md) separates documented capabilities from unverified gaps.

## Users and outcomes

| User | Job | Successful outcome |
| --- | --- | --- |
| Engineering lead | Find neglected work across teams | A prioritised list with owners, actual failures, age and expected action |
| Maintainer | Resolve an upgrade or bug safely | Small PR/MR, reproduction and validation evidence, no duplicate bot work |
| Platform engineer | Standardise recurring maintenance | Versioned policies, fleet onboarding, bounded campaigns and auditable exceptions |
| Reviewer | Decide whether a change can advance | Explainable rules, current provider checks, diff and test evidence on one page |
| Organisation administrator | Govern models, credentials and spend | Approved providers, limits, runner placement and scoped access |

Source provenance (human/agent/mixed/unknown) is optional metadata. It does not change the correctness standard or require unreliable AI-code detection. Imported repositories work regardless of how their code was written.

## First-release maintenance catalogue

| Recipe | Discovery | Work produced | Required validation |
| --- | --- | --- | --- |
| Repair dependency update | Existing Renovate/Dependabot PR/MR; failed required CI | Compatibility fixes linked to the original upgrade | Reproduce original failure; original checks pass at candidate commit; package/lock consistency |
| Repair build/test regression | Native CI failures after a commit or scheduled baseline | Minimal fix with cause and linked run | Failing baseline and passing candidate; never delete checks to pass |
| Fix bounded security finding | Imported advisory/scanner evidence, including dependency alerts | Patch for a supported finding; explicit uncertainty otherwise | Scanner rerun and targeted regression check; do not claim a security audit |
| Restore missing regression coverage | Reproduced defect or changed high-risk logic | Behavioural test protecting the demonstrated failure | Test fails against unfixed code and passes after repair where feasible |
| Maintain project instructions/docs | Repository changes invalidate owned instructions or links | Scoped docs change with changed symbols/commands referenced | Links/build commands validated; repository instructions respected |
| Remove bounded dead code | Deterministic unused-symbol/lint evidence | Small cleanup with constrained paths | Build, tests and static checks; no broad architectural rewrite |

Initial first-class validation presets: Go modules; JavaScript/TypeScript with npm, pnpm or Yarn; Python with project-specified commands. Other languages can use explicit build/validation recipes without a promise of stack-specific detection. Native forges and bots remain authoritative for ecosystem update resolution. Major framework/runtime migrations require a planned campaign and review; they are not silently classified as routine dependency repairs.

Security discovery uses existing provider/scanner findings and deterministic tools. Production error integrations such as Sentry are a later adapter; v1 can ingest authenticated structured findings and URLs. An integration import is never an instruction with elevated authority.

## Core workflow

1. Connect forge installation/instance and select repositories by group, team or explicit list.
2. Select model connection and runner pool; run capability and connectivity checks.
3. Discover existing bot configurations, active PRs/MRs, build commands, repository instructions and branch rules.
4. Present a baseline: monitored categories, gaps, current check failures, unsupported capabilities and estimated scheduling activity.
5. Apply organisation/team/repository policy; preview exactly which actions are allowed.
6. Discover and deduplicate findings. Queue permitted tasks within spend, concurrency and change limits.
7. Reproduce, plan, repair and verify in an isolated workspace. Publish one attributable change.
8. Evaluate provider gates and application policy. Request required native reviews; optionally queue/merge when eligible.
9. Observe or trigger approved CI/CD/GitOps deployment. Verify outcome from provider evidence.
10. Retain decision history. Suppress dismissed findings by fingerprint and expiry; reopen only on changed evidence.

## Portfolio behaviour

- Server-side filtering by forge, instance, team, repository, recipe, severity, activity age, bot owner, status and cost.
- Saved views are scoped to the organisation and optionally shared with teams.
- Bulk actions show an itemised preview and skip inaccessible/ineligible repositories with reasons.
- Campaigns select a versioned repository set and recipe, start with canaries, and halt remaining work when error or rollback thresholds are exceeded.
- Initial limits: one active write task per repository+target branch, three app-owned open changes per repository, two repair attempts per task. Owners can lower or raise within deployment limits.
- Fixed-window/concurrency budgets apply at organisation, team, repository, provider and runner-pool scope. Noisy organisations cannot monopolise shared workers.
- Repository health is an explanation of observable dimensions, not an unexplained score. Unknown and stale data are distinct from healthy.

## Product boundaries

Included in both editions: all three forges, model adapters, policies, recipes, job history, GUI, audit export, OIDC/RBAC, runner placement, merge/deployment orchestration and usage reporting. Hosting-specific provisioning, billing and support may wrap the OSS service; core maintenance is not disabled by a licence server.

Deferred until evidence requires them: generic workflow canvas, arbitrary user-loaded backend plugins, new code editor, new dependency solver, vector database, autonomous product feature generation, billing engine, SCIM, arbitrary direct production shell access, marketplace, cross-repository atomic releases and direct Kubernetes mutation.

Existing GitOps is the only Kubernetes change path. v1 deployment automation orchestrates an allowlisted existing pipeline or GitOps change; it never receives a cluster-admin credential.

## Acceptance journeys

- J01: organisation administrator connects all three forges and imports a 1,000-repository fixture portfolio without timing out or leaking inaccessible repositories.
- J02: maintainer finds and repairs an existing Renovate/Dependabot update without opening a duplicate version-bump PR.
- J03: reviewer sees baseline, patch, current native checks and policy explanation; a newly pushed commit invalidates old approval/evidence.
- J04: eligible change merges using native enforcement; unmet/unknown rules show a specific blocker.
- J05: merge causes one authorised staging deployment; required production approval remains pending; pipeline reruns are not triggered twice after a crash.
- J06: organisation pauses automation; pending local work stops at the next Reforge-controlled mutation boundary. Already-admitted native merges/deployments are cancelled where supported and reconciled; the GUI reports any action completed before cancellation.
- J07: exhausted subscription or API allowance pauses work; no silent switch to paid API or another provider.
- J08: customer-owned runner reaches a private Gitea/GitLab instance without exposing that instance or its credentials to another tenant.
- J09: self-hosted administrator restores database, encryption material and artifacts, upgrades one supported version, and resumes reconciled jobs.
- J10: a user cannot read another organisation's repositories, diffs, events, costs, artifacts or runner instructions through any API or GUI route.

## Initial engineering targets

Targets to validate before G5, not performance claims:

- 1,000 repositories per organisation and 10,000 per deployment in synthetic load tests; 100 concurrent runs across worker pools.
- Cached portfolio list/detail API p95 under 500 ms at 50 concurrent interactive sessions, excluding external provider calls.
- Signed webhook accepted/durably queued p95 under 2 seconds; foreground pages do not synchronously scan forges.
- Scheduler selects an eligible task within 60 seconds when quota and capacity are available.
- Crash/retry scenarios produce no duplicate published PR/MR, merge request, or pipeline invocation.
- Representative 20-case maintenance corpus reports actual outcomes: reproduced, fixed, validated, blocked and regressed. Ship no success-rate marketing claim from synthetic mocks.
- At least 16/20 bounded tasks should reach a correct tested fix in the selected supported model profile before enabling its autonomous pilot; zero policy bypasses is mandatory. Other models can remain manual/experimental without hiding their validation results.

## Evaluation metrics

Track median time to a validated fix, human review time, proportion of accepted fixes, duplicate work avoided, reverted changes, stale-evidence blocks, cost per accepted repair and time spent waiting on provider/rate limits. Count merged changes separately from deployed, verified outcomes. Avoid rewarding PR volume or lines of code.
