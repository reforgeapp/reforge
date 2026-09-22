# Reforge GUI specification

Updated: 2026-09-22. T29 rebuild authority; prior prototype direction is superseded.

## Rebuild authority

Apply [product-rebuild](../implementation/product-rebuild.md) across every route. Rundeck and AWX guide information density and operator workflow research only; no code or visual copying. Each route has one page title, one breadcrumb/context line and one primary toolbar. Remove duplicate breadcrumb/eyebrow headings and generic page/card introductions. Dense tables, filters and work surfaces carry context. Help is on demand through contextual links, versioned/searchable MkDocs and the in-app help drawer. Retain labels, accessibility, security notices, actionable errors, visible disabled reasons and unknown/stale states.

## Design stance

One application vocabulary and one connected journey: repository → finding → task → change → deployment. The operator can explain what happened without reading an agent transcript or visiting five dashboards. Native forge links remain available for reviews and controls that belong there.

Use a restrained desktop web interface: slate navigation, light neutral content, teal primary action, amber blocked/pending, red failure and green verified outcomes. Status always includes text/icon, never colour alone. Dense but legible tables; 14–16 px body text; consistent 8 px spacing; visible keyboard focus. No animated agent avatars, decorative fake activity, or opaque health scores.

React/TypeScript with accessible headless primitives and shared tokens. Responsive layout: desktop table+detail drawer; smaller widths show list rows and a full-screen detail view. Target WCAG 2.2 AA; keyboard navigation, labels, live-region announcements and reduced motion.

## Structural work surfaces

Shared hierarchy for inventory/workspace pages: organisation scope and one breadcrumb/context line; route title; one primary toolbar; work surface. Auth and short forms use appropriate controls. Work surface keeps list/filter state beside persistent detail context using split pane, route detail tabs, or URL detail. Modals are for short confirmation or focused editing only. Primary toolbar controls must be meaningful for route; tests query role and accessible name, never a universal class or one-toolbar count.

Runners use pool/runner inventory with search, state filters, pagination and persistent detail route or master-detail pane. Show only API-backed heartbeat, capacity/busy slots, trust, version, route capability and drain/enrolment state. Pool create/enrol is labelled form with progress, safe one-time handoff, actionable disabled reasons and recovery. Repository assignment uses names and searchable selectors, never comma-separated raw repository UUID input.

Connections use Forges, Models & agents, and Delivery tabs. Each tab has inventory and route detail/profile view. Create, capability test, rotate and revoke are guided forms. Credentials remain write-only. Model/agent detail names route, billing/qualification status, entitlement, protocol, version and last probe; unknown stays unknown. Avoid huge revoked-row table followed by essay card. Policies use named selectors and structured fields for simulation and rollout; raw JSON is advanced export/import only.

Findings, runs, changes and deployments use connected filterable lists, URL-addressable details and evidence timelines. Activity, artifacts, gates and provider links stay available while list context remains visible. Usage, audit and organisation routes use same inventory/detail pattern. All route states use persisted records; curated local demo data is labelled and separate from fixtures.

## Application shell

- Left navigation: Overview, Repositories, Findings, Runs, Changes, Deployments, Campaigns; administration group: Policies, Connections, Runners, Usage, Audit, Organisation.
- Top bar: organisation switcher, persistent team/portfolio filter, search and pause indicator. A user's organisation switch clears stale query caches and event subscription.
- Deep links include organisation and object ID. Search/filter/sort/page state lives in the URL. Back restores the prior list and selected row.
- Each object uses the same header pattern: title, provider/repository context, status, owner, last refreshed and permitted primary action.
- Detail tabs use familiar labels: Summary, Evidence, Activity, Configuration. Additional tabs only where necessary.
- Empty, loading, permission-denied, partial/stale-data, rate-limited and offline states are designed components, not raw errors.

## Screens

### Overview — `/org/:org/overview`

Show actionable counts: Needs decision, Running, Ready for review, Blocked, Verified deployments. Each count opens its filtered list. Include stale/unsynced repositories separately so an empty queue cannot imply everything is healthy.

Main content:

1. Attention queue: highest-impact findings, blocked changes, expiring/revoked connections, failed deployments. Columns: reason, repo/team, age, owner, next action.
2. Portfolio table: repository, forge/instance, monitored recipes, last successful validation, open work, current blocker.
3. Recent outcomes: verified fixes and deployments, with proof links. Trends use observed time/cost and state counts.
4. Capacity/spend strip: queued jobs, busy/available workers, period budget consumed/reserved, subscription quota if known.

No default token-speed chart, endless feed or log wall. Operators see the next decision first.

### Repositories — `/repositories`

Searchable, server-paginated inventory with team, forge, instance, stack, policy, bot manager, monitored/paused/archived and freshness filters. Bulk selection explicitly distinguishes visible rows from all matching rows.

Repo detail: baseline, detected stack/commands, existing Renovate/Dependabot configuration and active upgrades, effective policy, runner/model profile, findings, changes, deployment environments and branch-rule capabilities. Permission/protection data has a timestamp and refresh control. Never render an unavailable rule as disabled.

Import wizard:

1. Choose GitHub/GitLab/Gitea and instance.
2. Authenticate through supported app/OAuth/token flow. Show minimum required capabilities and exact granted access.
3. Browse eligible orgs/groups/repos with pagination; preview matched selection.
4. Assign team, runner pool and model profile. Probe private network reachability from that runner.
5. Detect existing bot work; show “Cooperate with Renovate” or “Cooperate with Dependabot”. If no bot exists, offer a reviewed Renovate onboarding PR; do not enable two managers silently.
6. Preview first scan, expected API use and configured activity limits. Begin with read-only discovery; chosen policy can enable repair/merge/deploy when prerequisites pass.

### Findings — `/findings`

Unified inbox with source, category, severity, evidence age, repository, owner, state and next action. Sources include bot PRs, CI, advisories, deterministic checks and optional imported reports. Group duplicates and show their source links.

Finding detail answers: What is wrong? What proves it? Is somebody already fixing it? What would Reforge do? Estimated scope and maximum budget are separate from speculative exact cost.

Actions: Queue repair, assign, snooze with expiry, dismiss with reason, link to existing work. Dismissal persists against the fingerprint and reopens only under documented changed-evidence rules. Bulk queue previews eligible/skipped rows and cost ceilings.

### Runs — `/runs` and `/runs/:id`

Table: recipe, repo, stage, elapsed time, model route, billed/estimated use, retry count, owner/campaign. Filters distinguish queued due to schedule/capacity/budget from execution failure.

Run detail stage rail: Discover → Reproduce → Plan → Repair → Validate → Publish. Separate change/deployment status follows publication; do not keep a repair process alive while waiting days for a reviewer.

Summary shows: goal, pinned starting revision, intended file scope, selected model and billing route, policy version, current action and outcome. Evidence contains baseline/candidate checks, diff, artifacts and full bounded logs on demand. Activity contains recorded decisions and events, not hidden model reasoning. Cancel explains that already-published provider actions remain visible.

### Changes — `/changes` and `/changes/:id`

Use “PR” for GitHub/Gitea and “MR” for GitLab in rows and detail headers. Show app/bot/human ownership, original dependency update, target branch, author, latest head, native check/review status, Reforge validation and effective policy.

Primary layout: diff and evidence in the main pane; eligibility panel alongside it. The panel lists each gate and its source:

```text
Candidate validation       Passed at a81d2f
Required CI checks         6/6 passed — GitLab
Code owner review          Awaiting platform team — GitLab
Target freshness           Current against e922b0
Reforge change policy      Low risk; paths within allowance
Merge route                Native merge train
Decision                   Waiting for code owner review
```

Unknown rules use “Cannot verify protection; automatic merge blocked”. No enabled merge control while gates are pending/unknown. Gate evaluation is refreshed on action. Show queue/train enrolment as pending external work, not merged. When evidence becomes stale, keep old results labelled with their SHA and explain which check is rerunning.

Actions: open native review, request eligible merge, pause app automation, rerun validation, inspect original bot PR. Application approval is labelled distinctly from a forge review.

### Deployments — `/deployments`

Deployment rows link repository, merged SHA, immutable build artifact, workflow, environment, native approval, rollout and verification status. Separate “pipeline succeeded” from “application verified”. A workflow without health evidence is “Completed; health verification unavailable”.

Detail has a delivery timeline and native links. Request deployment uses an allowlisted workflow/environment with a server-resolved artifact; no arbitrary shell field. Production approval lives in the native delivery system. “Request recovery” names the pre-approved rollback pipeline or GitOps revert, target artifact, evidence and required approvers. A database migration requiring manual recovery never gets an automatic rollback button.

### Policies — `/policies`

List organisation baseline, team overlays and repository exceptions. Editor tabs: Scope, Recipes, Changes, Models & spend, Merge, Deploy. GUI controls edit a versioned schema; YAML/JSON export is available for engineers.

The editor shows inherited values and the effective result, including which higher-level rule prevents loosening. Save creates a new version. Apply preview lists affected repositories and proposed gate changes. Simulation runs against stored findings/changes without calling tools or mutating providers.

Preset names: Observe, Propose fixes, Merge eligible fixes, Deliver to approved environments. These are editable configurations, not hidden feature tiers. Show exactly what each permits. Moving to a more permissive preset requires policy-admin role and an audit reason.

### Connections — `/connections`

Three tabs: Forges, Models & agents, Delivery integrations. Cards and table share health, scope, owner, last verification and action controls.

Model setup asks for connection type first:

- Provider API: OpenAI / Anthropic / Google.
- Self-hosted compatible endpoint: URL, protocol profile, model, assigned runner/network path and auth.
- Official agent on your runner: supported runtime, version, account/workspace, permitted deployment type and billing route.

Show capability test results (tools, structured output, streaming, usage), supported recipe roles and limitations. Subscription statuses: eligible, verification required, quota unknown, exhausted, disconnected. Never advertise a subscription route as “free” or auto-select a paid fallback.

Credentials are write-only. Show key fingerprint/last characters only when provider-safe, rotation date and revoke action. Credential entry is not copied to browser local storage, chat prompts or logs. Connection failure messages distinguish auth, permission, reachability, incompatible protocol and exhausted quota.

### Runners — `/runners`

Pool/runner inventory: hosted/customer-owned, tenant/team binding, trust level, runtime versions, private route capabilities, last heartbeat, busy slots and drain state. Enrolment provides an expiring one-time instruction; no permanent token embedded in a downloadable file. Run placement explains why a private repo or subscription agent requires the selected pool.

### Campaigns — `/campaigns`

Choose a recipe and repository filter; snapshot exact members. Preview exclusions, budgets, canary size, concurrency and stop thresholds. Start shows aggregate progress and per-repo evidence. Pause/cancel cannot roll back already-merged work; recovery uses individual recorded deployments. No cross-repo atomicity claims.

### Usage, audit and organisation

Usage: API cost versus subscription/credit consumption, observed versus estimated versus unknown, reserved versus settled, by team/repo/recipe/provider; configured budgets and pause conditions.

Audit: filterable immutable action history, actor and impersonation-free authority, policy version, before/after, provider action link; export respects permissions/retention.

Organisation: team membership, repository access, OIDC configuration, deployment limits and retention. Hosted billing portal is out of scope for the first OSS release.

## Coherence acceptance

- An operator can navigate from a failed Renovate update to its repair, tested diff, merge gates and deployed artifact without losing repository context.
- Global filters, terminology, timestamps, statuses and actions agree across every screen.
- Every blocked state has a reason, evidence freshness and an available next step; disabled controls explain the missing permission/gate.
- All screens use real persisted records in the built product. Synthetic prototype data never becomes a silent production fallback.
- No UI promise exceeds the provider capability record. A missing Gitea capability is visible without hiding the repository.
- Keyboard-only completion of onboarding, triage, policy preview and change inspection; contrast and focus verified with browser accessibility tooling.
- Error/retry, stale data, empty portfolio, provider outage, quota exhaustion and membership revocation are covered in the browser acceptance suite.

## Prototype

`prototype.html` is obsolete historical material. It is an offline synthetic interaction prototype, has no visual authority, is not a working service, makes no network calls and cannot evidence any integration.
