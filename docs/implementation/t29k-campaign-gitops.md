# T29k — Campaign and GitOps layout cleanup

Status: local complete, agent-reviewed, 2026-09-22. Commit `80fce9c`.

## Scope

- Own `web/src/app/CampaignsPage.tsx` and `web/src/app/GitOpsPage.tsx`; no style changes
  were required (the shared `.toolbar` layout already aligns the controls).
- Preserve every API operation, tab and disabled reason.

## Review before implementation

- Campaigns: the state filter sat in its own `subsection-actions` row and the Plan
  campaign action in a separate `row-actions` row; the planner then repeated the action
  as an `h2` plus a second close control.
- GitOps: the Configuration view repeated the active tab as an `h2`, and the Environment
  selector and the New environment input/Create draft action were on separate rows.

## Changes

- Campaigns: state filter and Plan campaign toggle share one compact `Toolbar`; the
  planner no longer repeats the heading or a duplicate close control (the toolbar button
  becomes Close planner).
- GitOps: Environment selector, New environment input and Create draft action share one
  aligned `Toolbar`; the Configuration `h2` and the Promotion history `h2` (which repeated
  the active tab) are removed. The promotions table keeps its `GitOps promotions` caption.

## Checks

- `npx playwright test campaigns.spec.ts gitops.spec.ts route-surface.spec.ts` — 13 passed
  (API operations, wizard steps, disabled reasons, tabs, one-title/one-toolbar, axe at
  390px).
- Visual review at 1440x900 and 390x844: populated campaigns with planner open, empty
  campaigns, GitOps configuration and promotion history; captures in `.local/visual/t29k`.
- Error states: the route visual harness asserts an actionable error for every route,
  including Campaigns and Deployments/GitOps — pass.
- Unrelated pre-existing failure: `bot-revalidation.spec.ts` "empty revalidation" focus
  assertion fails on the Changes route; it is outside this ticket's files and predates the
  change (concurrent in-progress shell work). Not addressed here.
