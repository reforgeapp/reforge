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

Current local checks: actual protected Gitea1.27.3 controller/runner/model test passed55.003s (`.local/repair-protected-restart.log`), including missing-review denial, stale configuration, exact native merge, restart reconciliation and replay. Real dependency upgrade passed153.769s; actual Go GUI repair passed1.8m. Queue PostgreSQL admission/final-C/pause/cancellation/uncertainty/restart/RLS contracts passed2.625s with explicit provider fixtures; these do not certify GitHub. Affected provider/transport/controller race tests pass.

Observer reads canonical native outcomes after restart and never repeats a merge. Policy, membership, connection or qualification revocation persists queue cancellation intent; native removal must be observed. GitHub and GitLab cancellation adapters converge without another mutation when the original admission is absent and the unchanged change remains open with automatic merge disabled. A provider can still execute concurrently; cancellation is not an absolute stop.

Companion dependencies derive from the original bot change in durable repair context. A pending, changed or unmerged companion blocks the original update. Fresh canonical reads must prove every published companion merged at the recorded head. Original-update checks, reviews, target enforcement and policy are evaluated separately on current native revisions. No success transfers from the companion to the bot update. Automatic original-update revalidation acceptance remains open.

GitHub App queue controller implements separate admission and execution decisions. Require the App-published `reforge/merge-policy` check, at least one other trusted native CI check, ALLGREEN grouping and one entry per merge group. Candidate C must be proven by the native queue and exact H/T commit parents. Admission publishes H policy success before native enqueue; execution refreshes policy, scope, companions, checks and rules before publishing success on C. Missing, changed, locked or unmergeable candidates remain blocked. Durable fenced intents prevent duplicate check POSTs after uncertainty/restart. Only canonical native merge observation marks merged. Other queue profiles remain disabled with an actionable reason.

Operator qualification must test the required App check on both pull_request and merge_group events, actor bypass restrictions, one-entry candidate proof, pause/dequeue races and native stale-candidate cancellation. Reforge is not claiming GitHub external certification. A successful policy check is the release boundary: cancellation can lose the race after publication; it does not revoke an already executing native merge.

GitLab final train gate remains disabled pending exact native candidate/pipeline/job binding and protected blocking manual-job qualification. A source-head commit status cannot establish that final gate. Remaining: GitLab train controller, original bot automatic revalidation, reviewed cooperation evidence verification and GitHub/GitLab external certification with dedicated accounts.

## GitLab train profile

Implemented locally; external GitLab certification remains required. Enable only after recording dated native qualification, the exact `.gitlab-ci.yml` SHA-256 and operational user ID as publisher for `reforge/merge-policy`. The source, target and native candidate must contain identical CI configuration. Source-head admission needs passing native CI; no synthetic H policy status is posted. Only an exact first candidate with two H/T parents can release; other train layouts stay blocked.

Use a standalone CI configuration without includes, retaining existing test/build jobs and this final job:

```yaml
reforge/merge-policy:
  stage: .post
  allow_failure: false
  inherit: false
  before_script: []
  after_script: []
  script: ["true"]
  environment:
    name: reforge-merge-policy
  rules:
    - if: '$CI_MERGE_REQUEST_EVENT_TYPE == "merge_train"'
      when: manual
```

The exact protected environment must grant deployment access only to the configured operational user. Overlapping wildcard access, additional environment approvals, unproven fields and mutable CI configuration block automation. This job releases the merge gate; it does not deploy the application. Native project approvals, protected target, pipeline-success requirement and enforced train without skip remain mandatory. The actor must lack protection bypass authority.

Controller reads the native train pipeline and every paginated job. It requires at least one successful nonoptional validation job, exact project/pipeline/C identity and a unique blocking final gate. A durable fenced intent precedes one native `play` call with no input/variable overrides. Lost responses retain uncertainty and never trigger another play; canonical job/merge observations continue. Pause attempts dequeue and reports the observed outcome. Native execution may win a cancellation race after release.

Protocol references: [merge trains](https://docs.gitlab.com/api/merge_trains/), [jobs API](https://docs.gitlab.com/api/jobs/), [blocking protected manual jobs](https://docs.gitlab.com/ci/jobs/job_control/), [protected environments](https://docs.gitlab.com/api/protected_environments/). Required external scenarios: exact deployed GitLab version/tier, H/T drift, pipeline replacement, actor bypass, protected-job access, missing approvals, lost play response and pause during merge. Local HTTP fixtures are not certification.

Automatic bot revalidation observes published companion repairs. Until the exact companion head is canonically merged, the original update remains waiting. After merge, Reforge reads the current original head/target and runs a fresh native merge evaluation under the repair author's current repository authority. CI refresh and branch updates remain with the native bot/CI. The observer never requests a merge. Revoked author access clears cached readiness; expired evidence renders stale. PostgreSQL contract covers original CI failure, refreshed revisions, restart, changed companion, tenant separation and revocation with zero provider writes. API: `GET /repositories/:repoID/changes/:changeID/revalidations`.
