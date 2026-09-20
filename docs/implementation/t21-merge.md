# Protected merge controller

Status: in progress; G3 open.

Controller persists immutable native snapshots and one active merge operation per repository/target. Each request refreshes native evidence, compares H/T/policy/rules/connection/configuration bindings, then rechecks current identity, scope, pause, policy and credentials before each native mutation. Lost responses retain reconciliation state; they do not trigger another merge. Cancellation after dispatch remains uncertain until canonical provider observation.

Repository merge configuration uses administrator-recorded qualification evidence: provider/server version, connection and inspector versions, evidence reference/hash, verification/expiry dates, exact-head and target-enforcement results. These are operator attestations, not automatic Reforge certification. Maximum validity:30 days. Credential rotation invalidates the binding. Reviewed bot-cooperation evidence and maintenance merge authority `reforge` are also required. Native rules remain mandatory regardless of this configuration.

Gitea protection inspection uses a separate same-tenant, same-origin, same-runner credential. Its transport permits only branch-protection reads. The mutation actor retains its own permissions and cannot borrow inspector privileges. Existing Gitea1.27.3 qualification supports protected fast-forward-only merging; trusted required status publisher enforcement, CODEOWNERS and native queues remain unsupported where provider enforcement is unverified.

Endpoints under `/api/v1/orgs/{org}`:

- GET/PUT `/repositories/{repo}/merge-configuration`; PUT requires version and administrator authority.
- POST `/repositories/{repo}/changes/{native}/merge-preview` with `method`.
- POST `/merge-operations` with immutable `gate_id` and UUID `idempotency_key`.
- GET `/merge-operations/{id}`.
- POST `/merge-operations/{id}/cancel` or `/reconcile`, with `If-Match`.

Current local checks: private connector/providers/Gitea race contracts pass; pure protection corpus passes; real PostgreSQL qualification rejection, RLS, cancellation contention and restart persistence pass1.353s. Changes GUI fixture browser2pass. These checks do not certify external provider instances.

Remaining: complete native merge controller acceptance, queue/train admission and execution-gate observation/cancellation, automatic reconciliation/pause propagation, companion ordering and original bot revalidation, configuration GUI, shared OpenAPI generation, stale-policy/credentials and lost native response service contracts. Required GitHub/GitLab external certification remains unavailable without dedicated accounts.
