# Protected merge controller

Status: in progress; G3 open.

Controller persists immutable native snapshots and one active merge operation per repository/target. Each request refreshes native evidence, compares H/T/policy/rules/connection/configuration bindings, then rechecks current identity, scope, pause, policy and credentials before each native mutation. Lost responses retain reconciliation state; they do not trigger another merge. Cancellation after dispatch remains uncertain until canonical provider observation.

Repository merge configuration uses administrator-recorded qualification evidence: provider/server version, connection and inspector versions, evidence reference/hash, verification/expiry dates, exact-head and target-enforcement results. These are operator attestations, not automatic Reforge certification. Maximum validity:30 days. Credential rotation invalidates the binding. Reviewed bot-cooperation evidence and maintenance merge authority `reforge` are also required. Native rules remain mandatory regardless of this configuration.

Gitea protection inspection uses a separate same-tenant, same-origin, same-runner credential. Its transport permits only branch-protection reads. The mutation actor retains its own permissions and cannot borrow inspector privileges. Existing Gitea1.27.3 qualification supports protected fast-forward-only merging; trusted required status publisher enforcement, CODEOWNERS and native queues remain unsupported where provider enforcement is unverified.

Endpoints under `/api/v1/orgs/{org}`:

- GET/PUT `/repositories/{repo}/merge-configuration`; PUT requires version and administrator authority.
- POST `/repositories/{repo}/changes/{native}/merge-preview` with `method`.
- POST `/merge-operations` with immutable `gate_id` and UUID `idempotency_key`.
- GET `/merge-operations?repository_id={repo}&change_id={native}` with cursor/limit pagination.
- GET `/merge-operations/{id}`.
- POST `/merge-operations/{id}/cancel` or `/reconcile`, with `If-Match`.

Current local checks: root local Gitea1.27.3 controller/runner/model repair and protected merge passed49.55s (`.local/repair-protected-merge.log`), including real approval and stale configuration denial. Later input-dependent repair and restart-fault variants remain under diagnosis; do not extend the earlier result to them. Provider/private transport race contracts pass. Current Changes/settings browser suite18passed8.3s, including keyboard/narrow layout, lost response, tenant switch, publisher deletion and companion order. Browser provider responses in that suite are explicit fixtures.

Observer reads canonical native outcomes after restart and never repeats a merge. Policy, membership, connection or qualification revocation persists queue cancellation intent; native removal must be observed. GitHub and GitLab cancellation adapters converge without another mutation when the original admission is absent and the unchanged change remains open with automatic merge disabled. A provider can still execute concurrently; cancellation is not an absolute stop.

Companion dependencies derive from the original bot change in durable repair context. A pending, changed or unmerged companion blocks the original update. Fresh canonical reads must prove every published companion merged at the recorded head. Original-update checks, reviews, target enforcement and policy are evaluated separately on current native revisions. No success transfers from the companion to the bot update. Full local dependency-upgrade and automatic original-update revalidation acceptance remain open.

Queue automation remains disabled until the final execution-gate controller and native candidate binding are implemented and qualified. Native cancellation and the GitHub App execution-check adapter are implemented building blocks, not a queue certification claim. GitLab merged-result pipeline gates require exact native candidate/job identity; an external source-head status alone is insufficient.

Remaining: queue/train admission and final execution gates; original bot automatic revalidation; strengthened real dependency and restart scenarios; reviewed bot-cooperation evidence verification; GitHub/GitLab external certification with dedicated accounts.
