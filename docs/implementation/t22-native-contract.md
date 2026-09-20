# T22 native delivery contract

Implementation: `internal/deployment`, `internal/forge/{github,gitlab}/delivery*`. Local HTTP/PostgreSQL/browser contracts are separate from live installation certification. Dedicated GitHub/GitLab accounts, the required native plan/tier, workflow configuration, non-bypass actor and protected environments are still required for external certification. Signing and callback commands: [evidence runbook](t22-evidence.md).

## Configuration and authority

Owner/admin binds a logical environment to one repository/native environment, full workflow ref, immutable workflow SHA, file path/content SHA-256, fixed inputs, provenance/health Ed25519 public keys, observation window and separately configured recovery workflow. Native environment identity cannot be aliased to evade serialization. Database uniqueness and the qualified native workflow serialize execution.

Qualification identifies provider/server/connection version and dated evidence, expires within30days, and attests pinned inputs, native enforcement, environment serialization and no bypass. Operator attestation is distinct from Reforge's external certification. Pin and review all included/called configuration and workflow dependencies. Configuration, connection, policy or native-rule changes invalidate admission.

Preview verifies the canonical merged source change, signed artifact provenance, workflow identity/ref/content hash and native rule hash. Artifact source SHA and workflow SHA are separate identities. Both environment and workflow require explicit policy allowlists. `deployment_admission` requires proof that the native system enforces its gates; ordinary deployment evaluation still requires native approvals. An ancestor explicitly requiring pre-dispatch approvals remains blocking. Reforge never calls native approval APIs.

## GitHub Actions

Dispatch uses native workflow ID, pinned ref and fixed `reforge_operation`, `reforge_workflow_sha`, `reforge_source_sha`, `reforge_artifact_digest`, `reforge_environment` inputs. The reviewed workflow checks its actual revision against `reforge_workflow_sha` before side effects, consumes the exact digest/environment, uses native environment gates and serializes that environment. Set run-name exactly `reforge:${{ inputs.reforge_operation }}`. Mutable included actions/configuration or privileged bypass credentials are ineligible.

A204 response remains uncertain until exactly one native run is found. Match repository, workflow ID/path, ref, workflow SHA, `workflow_dispatch`, correlation and attempt1. Pending approval remains `awaiting_gates`; native success remains `completed_unverified` until authenticated health passes. Custom protection rules without an inspection profile are disabled. Required reviewers must prevent self-review.

Native APIs: [Workflows](https://docs.github.com/en/rest/actions/workflows), [Workflow runs](https://docs.github.com/en/rest/actions/workflow-runs), [Deployment protection](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/control-deployments).

## GitLab CI/CD

Bind project, exact configuration path/hash, protected branch/tag and environment. Fixed variables: `REFORGE_OPERATION`, `REFORGE_WORKFLOW_SHA`, `REFORGE_SOURCE_SHA`, `REFORGE_ARTIFACT_DIGEST`, `REFORGE_ENVIRONMENT`. The reviewed pipeline verifies its own revision before promotion, consumes the digest, uses native protected-environment approvals and resource serialization. Native `api` source, variables, project, SHA, ref and jobs provide correlation. Child pipelines, retried jobs, ambiguous environment wildcards and incomplete gate metadata block this profile.

Native APIs: [Pipelines](https://docs.gitlab.com/api/pipelines/), [Jobs](https://docs.gitlab.com/api/jobs/), [Protected environments](https://docs.gitlab.com/api/protected_environments/).

## Provenance, health and recovery

Promotion requires a prebuilt immutable digest. A workflow that builds after invocation must complete its build/provenance stage before requesting promotion through the configured deployment workflow; build success alone grants no authority. Signed provenance binds organisation/repository/source SHA/digest/build identity and bounded issuance/expiry.

Native run success does not assert deployed artifact identity. Health comes from the configured trusted reporter and binds organisation/deployment/logical environment/configuration version, source/digest, native run/attempt, actual revision, configured criteria, timestamp and nonce. It must remain fresh through the actual observation window. Bad signature, wrong identity, changed nonce payload, stale/regressing time and replacement attempts are rejected. A successful pipeline without attributed health never becomes healthy.

Durable intent precedes dispatch. Lost response/restart observes the original correlation without repeating dispatch. One normal native cancellation request is persisted first; pause/revocation uses the same path. Uncertain cancellation is observed without repeating POST. Native execution may finish before cancellation; canonical outcomes win. Recovery is a distinct operation using a separately configured workflow and previously verified healthy immutable artifact. Failed history remains failed.

Observe-only imports bind a known native run to the configured workflow and signed source/artifact provenance. They use read APIs exclusively and cannot dispatch, approve or cancel native execution. Attributed health does not confer native approval authority. Gitea Actions delivery remains actionably unsupported; T23 supplies the portable protected GitOps path. No direct Kubernetes mutation endpoint exists.
