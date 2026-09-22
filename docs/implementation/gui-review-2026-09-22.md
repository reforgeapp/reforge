# GUI review — 2026-09-22

Review scope: direct inspection of `.local/visual/runners-1440.png`, `connections-1440.png` and `runs-1440.png`, plus source/spec review. No fresh live-browser review of every route was performed. This report records findings; it does not certify T29.

## Findings

- `c8f9a61` is described as ground-up, but AppShell retains same aside/navigation/main organization. Changes are palette, icons, brand tagline, helper/group text and CSS. No materially changed resource hierarchy is demonstrated.
- Runners view lacks searchable/filterable inventory and operational heartbeat/capacity/trust summary. Current pool pagination is cursor-based `Load more`, not a complete searchable inventory. Repository selection uses raw/comma-separated IDs. Detail is modal-oriented instead of persistent operator context.
- Connections presents a large revoked-row table before profile workflows. Forge, model/agent and delivery profile management is not a compact tabbed operational surface.
- Policies still use evidence/rollout/binding JSON for simulation. Named structured selectors and guided controls are required for primary workflows; JSON may remain advanced export/import.
- Runs screenshot is empty despite populated-state claims. Existing visual capture waits 700ms and route surface tests count `.repository-toolbar`; these do not assert readiness, nonempty data or meaningful actions.
- Three reviewed PNGs cannot establish all-route coverage. Existing metadata and axe/count checks are supporting evidence only; no fresh browser review occurred here.

## Required correction

Reopen T29 and execute T29a–f: old/new IA map and wireframes; shared list/detail shell; Runners/Connections vertical slice; all remaining routes including Overview, Repositories, Findings, Runs, Changes, Deployments, Campaigns, Policies, Usage, Audit, Organisation and auth/help; workflow/accessibility/responsive state coverage; comparative visual review. Preserve API/security and backend gains. Do not impose a rigid toolbar rule on sign-in.

Acceptance captures same route/data/state before and after at 1440x900 and 390x844, with route readiness, browser, commit/build and served asset identity recorded after cache reset. Measure representative task completion and click path. Curated persisted local demo records must be labelled separately from fixtures; UUID fixture walls and fake production fallback fail the design baseline.

## Status limits

T31 extraction/validation code is reported integrated; focused regression was not independently run in this review. Local OIDC/TLS pass is reported; hosted/customer OIDC remains external. Existing load notes are insufficient to establish their exact execution/browser coverage. T28 remains partial.
