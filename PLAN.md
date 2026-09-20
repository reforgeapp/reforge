# Reforge — build plan

Status: build-ready specification; product implementation has not started.
Updated: 2026-09-20. Product: Reforge. Directory: `/home/mnorris/repos/reforge`. Go package github.com/sindef/reforge.

## Agreed requirements

| ID | Requirement | Release commitment |
| --- | --- | --- |
| R01 | Apache-2.0 open-source application with a coherent GUI | Complete core workflow available as OSS |
| R02 | Go backend with Gin | Gin HTTP API; scheduler, policy evaluation, adapters, orchestration and runner supervisor in Go |
| R03 | Hosted multi-tenant service and self-hosted edition | Same source, schema, API and GUI; deployment configuration differs |
| R04 | Engineering teams with large repository portfolios | Organisation/team/repository scopes, bulk onboarding, filters, bounded campaigns and fair scheduling |
| R05 | GitHub, GitLab and Gitea | All three required for v1; cloud and supported self-managed installations |
| R06 | Raise pull/merge requests | Small, attributable changes with evidence and links; preserve native provider terminology |
| R07 | Respect branch protection | Provider rules plus stricter application policy; unknown enforcement prevents automatic merge |
| R08 | Cooperate with Renovate and Dependabot | Adopt existing work, avoid duplicate upgrades, repair failures under explicit branch ownership rules |
| R09 | OpenAI, Claude, Google and self-hosted models | Native API adapters plus compatible self-hosted interfaces; capability-tested configuration |
| R10 | Prefer subscription routes where permitted | Official customer-owned agent integrations only where documented terms and deployment topology permit |
| R11 | Policy-controlled merge and deployment | Separate permissions and stages; freshness checks, native gates, existing CI/CD/GitOps |
| R12 | Explicit Luna/Astra implementation plan | Bounded tickets, dependencies, exclusive file ownership and Astra review of sensitive work |
| R13 | Persistent, build-ready specification | This index, product/UX/architecture/contracts/policy plans, research and executable task instructions |

## Read in this order

1. [Product scope](docs/product.md): users, outcomes, maintenance catalogue and v1 boundaries.
2. [GUI specification](docs/design/gui.md): screens, navigation and complete workflows.
3. [Architecture](docs/architecture.md): services, isolation, storage and deployment.
4. [Contracts](docs/contracts.md): records, APIs, events and adapter contracts.
5. [Policies and lifecycle](docs/policies.md): branching, bot coordination, merge and deployment state machines.
6. [Implementation backlog](docs/implementation/backlog.md): task IDs, dependencies and acceptance checks.
7. [Worker runbook](docs/implementation/agents.md): implementation invocation and Luna/Astra allocation.
8. [Decisions](docs/decisions.md): accepted choices, assumptions and gates.

Open the [offline GUI prototype](docs/design/prototype.html) in a browser to explore the visual direction and core workflow. It uses synthetic data and simulated actions. The [acceptance matrix](docs/implementation/validation.md) defines the implementation evidence required at release.

Research inputs:

- [Competitor evidence](docs/research/competition.md)
- [Forge capabilities](docs/research/forge-integrations.md)
- [Model and subscription integrations](docs/research/model-integrations.md)

## Delivery sequence

| Gate | Deliverable | Required proof |
| --- | --- | --- |
| G0 | Contracts and runnable foundation | Versioned API; tenant isolation; session/RBAC; migration and development environment |
| G1 | Read-only portfolio across all three forges | Onboarding, inventory, bot detection, branch-rule visibility, model capability probes |
| G2 | End-to-end repair with human-reviewed PR | Reproduce failure, repair in isolation, validate, publish once, show evidence in GUI |
| G3 | Policy-controlled merge | Per-provider protection contract tests; stale evidence invalidation; queue/train handling |
| G4 | Policy-controlled deployment | Existing CI/CD/GitOps workflow, native approvals, exact artifact attribution, failed rollout handling |
| G5 | Portfolio-scale v1 release | Hosted isolation and self-hosted install; all R01–R13 checks; accessibility and recovery tests |

G2 is a useful private alpha, not fulfilment of the complete v1 request. G5 requires GitHub, GitLab and Gitea, all four model families, and policy-controlled merge/deployment support within documented capabilities. Unsupported server versions remain visible and read-only/PR-only with a reason.

## First demonstration

Connect a GitHub organisation, a GitLab group and a Gitea organisation. Import repositories. Detect an existing Renovate PR whose build failed. Show the failing baseline, propose an independently valid compatibility repair on an app-owned branch, run checks, and present the linked change and merge order in the GUI. Revalidate the original bot update after the repair merges. Reconcile required reviews and branch rules, merge only when eligible, then observe a pre-authorised staging deployment. Show an intentionally blocked production deployment awaiting its native approval. If the repair cannot stand alone, require the reviewed handoff described in the policy specification.

## Requirement ownership

| Requirement | Primary tickets |
| --- | --- |
| R01 OSS and GUI | T01, T04, T17, T20, T24, T26 |
| R02 Go/Gin | T01 and backend tickets T02–T03, T05–T16, T18–T19, T21–T23, T25 |
| R03 Hosted/self-hosted | T02–T03, T07, T26–T28 |
| R04 Large portfolios | T05, T11, T17, T25, T28 |
| R05 All three forges | T08–T11, T21–T23, T28 |
| R06 PR/MR publication | T08–T10, T19–T20 |
| R07 Branch protection | T06, T08–T10, T21, T27 |
| R08 Dependency bots | T11, T18–T19, T21 |
| R09 Model families | T12–T15, T19, T28 |
| R10 Permitted subscriptions | T03, T07, T16, T17, T26, T28 |
| R11 Merge/deployment | T06, T21–T25, T27–T28 |
| R12 Worker plan | Worker runbook; applies to T01–T28 |
| R13 Persistent specification | This planning package; T01 contract freeze and T28 handoff |

## Implementation readiness

- Product name: Reforge. Module/import namespace is provisional until a repository remote is chosen.
- Licence: Apache-2.0. Add the standard licence and applicable dependency notices during foundation.
- Live provider credentials and dedicated test organisations are required only for integration certification; recorded fixtures cover local development.
- A provider's consumer subscription is never treated as a generic API credential. Unverified subscription routes remain disabled while API integration ships.
- Hosted execution must pass the isolation gate before untrusted customer repositories run.
- This task produced specifications and an offline synthetic GUI prototype. Service implementation begins with T01; the worker runbook contains the launch instruction.

Planning checks completed: internal document links and Markdown fences pass; all 28 ticket dependencies form an acyclic graph; every R01–R13 requirement has build ownership; independent Astra review found no remaining critical/high specification issues. Prototype JavaScript passes `node --check` and uses no external script/style dependencies. Browser rendering and live provider/inference integrations have not been tested in this planning environment.
