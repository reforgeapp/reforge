# Product rebuild specification

Status: planning authority for T29–T31. Updated 2026-09-21. No implementation or certification claimed.

## Scope

Rebuild GUI across shell, navigation, design system, forms, tables, states and every route. Existing prototype is historical only. Use Rundeck project/job/activity and AWX resource/task workflows as research references for dense operational work surfaces: [Rundeck getting started](https://docs.rundeck.com/docs/manual/03-getting-started.html), [Rundeck activity](https://docs.rundeck.com/docs/manual/08-activity.html), [AWX user guide](https://docs.ansible.com/projects/awx/en/24.6.1/userguide/index.html). Do not copy code, branding or visual assets.

GUI supports actual SaaS and OSS/self-hosted workflows with persisted records. GUI actions and documented provider integrations are primary. Tenant configuration and operational tasks use forms and guided workflows, not raw JSON. Infrastructure install/upgrade, host preparation and official provider login/native approval steps may require documented operator or provider handoff outside the browser; GUI must show prerequisites, status, links and remediation. SQL and curl remain diagnostic/admin escape hatches only.

## Hard interface rules

- One page title, one breadcrumb/context line, one primary toolbar per route.
- No duplicated breadcrumb plus eyebrow; no generic intro or explanatory header above every card.
- Dense tables, filters, row actions, detail panes and timelines carry operational context.
- Help appears on demand through field help, searchable MkDocs and in-app help drawer.
- Every form keeps explicit labels, accessible descriptions, security notices, validation and specific next-step errors.
- Unsupported, unverified, stale or blocked actions visibly disabled with reason and remediation.
- Unknown entitlement, usage, provider enforcement or health never renders as success.

## Route coverage

| Route | Work surface | Required real workflow |
| --- | --- | --- |
| Overview | attention/portfolio tables, capacity strip | triage stale/blocked work and open filtered records |
| Repositories | paginated inventory, import toolbar, detail tabs | connect GitHub/GitLab/Gitea, preview, assign, import |
| Findings | queue table and finding drawer | inspect evidence, assign, snooze, repair or link bot work |
| Runs | run table and stage timeline | start, observe, cancel/retry, inspect usage/evidence |
| Changes | PR/MR table, diff/evidence pane, gate matrix | inspect native checks, request review/merge, reconcile stale evidence |
| Deployments | delivery table and timeline | observe/trigger allowlisted native pipeline or GitOps, recover |
| Campaigns | pinned membership table, canary controls | preview, start, pause, stop, inspect fairness |
| Policies | inheritance editor, simulation and preview tables | approve versioned policy and understand denials |
| Connections | forge/model/agent/delivery tables and forms | native auth, capability probe, rotation/revocation |
| Runners | pool table, enrol/drain controls | enrol customer runner, route probe, drain |
| Usage | settled/reserved/unknown ledger, budget create/edit form | inspect and change budgets with policy/audit; no invented zero usage |
| Audit | filterable event table/export | inspect actor, policy, provider action, evidence |
| Organisation | teams, access, OIDC, retention forms | administer scope and security settings |

## Operation/help matrix

| Operation | GUI proof | On-demand help |
| --- | --- | --- |
| Forge onboarding | provider auth, scope preview, pagination and import result | scopes, private route, revocation |
| Model/agent setup | API, compatible endpoint or qualified official runtime | billing route, entitlement, protocol, custody |
| Authentication/bootstrap | OIDC login, self-hosted one-time bootstrap and session/revocation states | host prerequisites, recovery and revocation |
| Custom command profile | versioned admin profile with executable, argv, image digest, approval | allowlist, image provenance, protocol and cancellation |
| Repair | persisted baseline, plan, candidate, validation, publication | isolation, limits, evidence freshness |
| Merge | native checks, approvals, rule source, exact head | enforcement and unknown-state behavior |
| Deployment | source SHA, artifact digest, workflow/run, approval, health | native approval and recovery limits |
| Install/upgrade | clean Compose wizard/checklist and diagnostics | host prerequisite, backup/restore, migration recovery |
| Security | visible scope, secret custody, runner trust, blocked reason | threat model, incident/revocation procedure |

## Visual regression

Capture route screenshots at desktop and 390px widths, keyboard focus, empty/loading/error/blocked/stale states and populated workflows for each route family. Runner regression must show exactly one page title, one toolbar and one dense table. Compare against approved T29 baselines with route/runtime metadata. Prototype screenshots cannot satisfy acceptance.

## Agent and command flows

Agent choices are distinct: Claude Code, Codex and Google Antigravity CLI `agy`; `agy` is not Gemini CLI. Custom command runner is a versioned administrator-approved profile with executable, fixed argv template, container image digest, protocol version and declared input/events/output/cancel/exit/usage behavior. Tenants cannot submit arbitrary shell or executable paths. Qualification requires entitlement, headless behavior, container topology, approval interception and exact runtime/version evidence. Unknown/unverified status is visibly disabled and actionable. Credentials stay isolated; no subscription token becomes an API key.

## Documentation

T30 delivers versioned/searchable MkDocs end-user, administrator and operator docs, container build/serve and in-app deep links. Reference: [MkDocs](https://www.mkdocs.org/). Docs include concrete GUI setup, enrolment, auth, capability probe, runtime selection, quota/usage and revocation procedures for Claude Code, Codex, `agy` and approved custom profiles; clean Compose install and hosted GitOps; provider auth and native approvals; runner host prerequisites; infrastructure bootstrap exceptions; container validation; and disabled-feature remediation. Documentation may describe permission-blocked routes and handoffs, but does not claim those flows are live or certified.
