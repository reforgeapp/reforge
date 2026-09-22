# T29a — information architecture and wireframes

Status: 2026-09-22. Authority: `docs/implementation/product-rebuild.md` and
`docs/design/gui.md`. Rundeck project/job/activity and AWX resource/task workflows are
interaction research only; no code, branding or assets are copied.

## Shell hierarchy

```
[organisation scope]  [breadcrumb / context line]
[route title]                                     [on-demand Help]
[one primary toolbar: scope + filters + primary action]
[work surface]
```

The topbar carries organisation scope and the single breadcrumb/context line. The route
renders exactly one title, one primary toolbar for inventory/workspace routes, and a work
surface. Auth pages and short confirmation forms are exempt from the one-toolbar rule and
use the control appropriate to the task.

## Route hierarchy and interaction

| Route | Inventory / list | Persistent detail | Evidence / activity |
| --- | --- | --- | --- |
| Overview | attention queue, portfolio, capacity | opens the filtered list | recent outcomes |
| Repositories | paginated inventory + import toolbar | URL `?repository=` split detail | baseline, discovery, maintenance, changes |
| Findings | filterable queue | URL `?finding=` detail | evidence, blockers, repair preview |
| Runs | filterable run table | URL `?run=` detail | stage rail, frozen checks, artifacts, live events |
| Changes | PR/MR table | URL `?change=` detail | diff, gate matrix, native links |
| Deployments | delivery table | URL `?deployment=` detail | timeline, artifact, native approval, health |
| Campaigns | campaign history | URL `?campaign=` detail | members, canary outcomes, fairness |
| Policies | version history | scope editor panel | effective policy, simulation, rollout |
| Connections | tabs: Forges, Models & agents, Delivery | URL `?connection=` profile detail | capability, qualification, entitlement, routes |
| Runners | searchable pool inventory | URL `?pool=` pool detail | runners, heartbeat, capacity, routes, drain |
| Usage | usage ledger | budget/route panel | settled/reserved/unknown |
| Audit | filterable events | URL `?event=` detail | actor, policy, provider action, data |
| Organisation | teams + members | inline editors | scope, retention, identity handoff |

Detail is URL-addressable so filters, sort and page survive. On desktop the inventory and
detail render as a split pane where the operator must compare rows; on narrow widths the
detail becomes the full view with a back control. Modals are reserved for short
confirmation or focused create/edit forms.

## Runners pilot (wireframe)

```
Runners                                            [Help]
[ Search pools | State | Refresh ]                 [Create pool]
+----------------------------+  +--------------------------------------+
| Pool        Runners  Busy  |  | Pool: hosted-linux        [draining] |
| hosted-linux   3     1/4   |  | trust customer-owned · version 2     |
| build-mac      0     0/2   |  | last heartbeat 12s ago · routes 4    |
| gpu            1     1/1   |  | Repositories: payments, ledger        |
+----------------------------+  | Runners: 3 (1 busy)                  |
                                |  [Enrol runner]  [Drain]  [Revoke]   |
                                +--------------------------------------+
```

Pool create/enrol is a labelled form with a searchable, named repository picker (never
comma-separated IDs), a progress state, and a one-time token handoff with copy-safe
instructions and expiry. Disabled actions name the missing prerequisite.

## Connections pilot (wireframe)

```
Connections                                        [Help]
[ Forges | Models & agents | Delivery ]            [Add connection]
+----------------------------+  +--------------------------------------+
| Name        Kind    State  |  | gitea-prod · forge/gitea  [healthy]  |
| gitea-prod  forge   healthy|  | endpoint, scope, last verified       |
| openai      model   healthy|  | capability probe results             |
| codex       agent   disabled| | [Test] [Rotate] [Revoke] [Private]  |
+----------------------------+  +--------------------------------------+
```

Each tab lists only its kind. Model/agent detail names route, billing/qualification
status, entitlement, protocol, version and last probe; unknown stays unknown. Credentials
stay write-only. A large revoked-row wall and a trailing essay card are replaced by the
tabbed inventory + profile detail.

## Responsive and state rules

- 1440x900: inventory and detail side by side; dense tables.
- 390x844: single column; detail replaces the list with a back control; toolbars stack;
  tables scroll horizontally inside a labelled, focusable region.
- Every route renders loading, empty, error, blocked and stale states from persisted
  records, with a specific reason and a next step. No fake production fallback.
- Representative persisted records are used; explicit fixtures stay labelled.

## Qualitative acceptance (against Rundeck/AWX research)

- The next operator decision is reachable in one click from the overview.
- A resource can be found by name/state, opened without losing filters, and acted on from
  a persistent detail panel.
- Health, capacity, trust, version, entitlement and drain are real values or visibly
  unknown; nothing is inferred from login.
- Disabled controls explain the missing permission, gate or prerequisite.
