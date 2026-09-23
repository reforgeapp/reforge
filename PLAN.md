# Reforge — implementation plan

Status: frozen source `675b0fd` passed production-browser193/15 and local amd64 image/install/recovery acceptance. T29 invitation/lifecycle fixes locally verified; remaining release/provider qualification stays open. Historical planning pauses below are superseded by current work status.
Current open defects and acceptance actions: [work status](docs/implementation/work-status.md).
T31a protocol handling and T29s repository-scope status are locally complete. Positive protected-merge acceptance, hosted isolation and external certification remain open; G5 is not passed.
Updated: 2026-09-24. Product: Reforge. Directory: `/home/mnorris/repos/reforge`.

## Release requirements

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

## Product-wide interface gate

Every route uses one page title, one breadcrumb/context line and one primary toolbar. No repeated eyebrow and breadcrumb, generic introductory copy, or explanatory header above every card. Tables, filters, dense toolbars and work surfaces carry the task. Help is on demand through contextual links, searchable MkDocs and an in-app help drawer. Labels, keyboard access, focus, field descriptions, actionable errors, security notices and stale/unknown states remain mandatory.

Rundeck operator-console information density and AWX operational workflows are reference material for hierarchy and interaction only. No code, branding or visual assets are copied. `docs/design/prototype.html` is obsolete synthetic history with no visual authority; `docs/implementation/product-rebuild.md` and `docs/design/gui.md` govern the rebuild.

## Documentation and distribution commitments

MkDocs end-user, administrator and operator documentation is a dedicated release deliverable. Versioned, searchable docs are built and served in containers and linked from contextual in-app help. Every component is containerised: Go control plane/GUI, controllers, migrator, runner, agent profiles, validation images, docs and local dependencies. Compose supports clean installation without host Go/Node/Python or repository-specific `/tmp` paths; hosted operation is versioned GitOps. Native provider authentication and approvals remain required. Chosen runtime host prerequisites and privilege model are explicit; Docker is not tenant isolation. No API/task container receives a Docker socket, credentials are never baked into images, and direct SQL/curl/JSON is never a primary workflow.

## Read order

1. [Product](docs/product.md) and [rebuild specification](docs/implementation/product-rebuild.md)
2. [GUI](docs/design/gui.md)
3. [Architecture](docs/architecture.md)
4. [Contracts](docs/contracts.md)
5. [Policies](docs/policies.md)
6. [Backlog](docs/implementation/backlog.md), [validation](docs/implementation/validation.md), [progress](docs/implementation/progress.md)
7. [Worker runbook](docs/implementation/agents.md) and [decisions](docs/decisions.md)

## Delivery gates

| Gate | Deliverable | Required proof |
| --- | --- | --- |
| G0 | Contracts and runnable foundation | Versioned API, tenancy, sessions/RBAC, migration, clean container build |
| G1 | Read-only portfolio | All three forges, inventory, bot detection, capability probes |
| G2 | Human-reviewed repair | Reproduction, isolated repair, validation, one attributable publication |
| G3 | Policy-controlled merge | Provider protection contracts, fresh evidence and queue/train handling |
| G4 | Policy-controlled delivery | Existing CI/CD/GitOps, native approvals, artifact attribution and recovery |
| G5 | Portfolio-scale release | Full GUI rebuild, docs, clean install, hosted isolation, OSS/self-hosted checks, R01–R13 |

G2 is a useful private alpha, not complete v1. G5 requires GitHub, GitLab and Gitea, all four model families, qualified agent routes and policy-controlled merge/deployment support within documented capabilities. Unsupported versions remain visible and read-only/PR-only with reason.

## First demonstration

Connect a GitHub organisation, GitLab group and Gitea organisation. Import repositories. Detect an existing Renovate PR whose build failed. Show failing baseline, propose an independently valid compatibility repair on an app-owned branch, run checks, present linked change and merge order in GUI, revalidate original bot update, reconcile reviews and branch rules, merge only when eligible, observe pre-authorised staging deployment, and show production deployment blocked pending native approval. If repair cannot stand alone, require reviewed handoff under policy.

## Requirement ownership

| Requirement | Primary tickets |
| --- | --- |
| R01 | T01, T04, T17, T20, T24, T26, T29, T30 |
| R02 | T01–T03, T05–T16, T18–T19, T21–T23, T25, T31 |
| R03 | T02–T03, T07, T26–T28, T31 |
| R04 | T05, T11, T17, T25, T28, T29 |
| R05 | T08–T11, T28, T29 |
| R06 | T08–T10, T19–T21, T29 |
| R07 | T06, T08–T10, T21, T27, T29 |
| R08 | T11, T18–T19, T21 |
| R09 | T12–T15, T19, T28, T31 |
| R10 | T03, T07, T16, T17, T26, T28, T31 |
| R11 | T06, T21–T25, T27–T29 |
| R12 | `docs/implementation/agents.md` and ticket records |
| R13 | This package, T01, T28–T31 |

## Planning checks

Historical planning note: this records the earlier pause and T29 reopening, preserves implementation evidence, and adds explicit acyclic T29a–f children. The current T29 status is maintained in `docs/implementation/progress.md` and the dated GUI review. No certification, entitlement or release claim is made here. Root/Astra performs final architecture/review; Luna owns assigned slices under the current handoff.

Research inputs: [competition](docs/research/competition.md), [forge capabilities](docs/research/forge-integrations.md), [model integrations](docs/research/model-integrations.md). Official planning references: [Rundeck getting started](https://docs.rundeck.com/docs/manual/03-getting-started.html), [Rundeck activity](https://docs.rundeck.com/docs/manual/08-activity.html), [AWX guide](https://docs.ansible.com/projects/awx/en/24.6.1/userguide/index.html), [Antigravity headless CLI](https://antigravity.google/docs/cli/headless/), [MkDocs](https://www.mkdocs.org/). These references support interaction/protocol planning only; they do not certify entitlement, isolation or provider support.
