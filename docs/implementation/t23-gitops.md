# T23 GitOps operations

Open **Deployments → GitOps promotions**. Configure source and delivery repositories, target branch, manifest path, RFC6901 field pointer, registry image repository, provenance/health public keys and observation criteria. Owner/admin configures; maintainers with access to both repositories may promote. Both repository policies must allow the environment and `gitops:<environment>` workflow. Where recipes are allowlisted, include `gitops-manifest`.

The source change must already be merged. Supply its native change number, exact merge SHA and immutable `sha256:` artifact digest with signed build provenance. Preview shows the original field and replacement. Supported manifests contain one YAML/JSON document and a single-line string at the configured pointer. Duplicate keys, aliases, tags, merge keys, multiline targets, excessive depth and ambiguous edits fail closed.

Request creates an owned delivery branch and one native PR/MR. The controller checks the native commit parent and complete Git tree, including the exact changed file. Stage and publication have separate durable dispatch identities. Unknown outcomes require **Observe**, then **Continue** only after the original candidate is recovered. Repeating a request preserves its idempotency identity; no automatic second publication follows an ambiguous response.

Configure the delivery repository's existing **Changes → Merge settings** first. Use a qualified non-bypass merge actor, independent protection reader where required, native checks/reviews and repository merge authority. **Protected merge preview** reports blockers; the merge method must be supported by native rules and policy. Native approvals happen in the forge. Reforge rechecks source access, delivery access, original requester's authority, both policies, artifact provenance and target identity immediately before effects. Direct merge endpoints enforce the same GitOps guard.

A merged delivery change becomes `completed_unverified`. The existing GitOps reconciler applies it through its normal process. Reforge has no Kubernetes write client or cluster mutation path.

## Attributed health

Use a trusted reconciler/observer to measure the actual delivery revision, source revision, artifact digest and configured criteria. Keep the signing key in that observer's secret store. [Offline signing](t22-evidence.md) documents key generation and `--kind gitops-health`; signing alone does not prove rollout success.

POST the exact signed document to `/api/v1/gitops-health/{orgID}/{promotionID}` with `X-Reforge-Signature`. Required document fields:

```json
{
  "org_id": "organisation UUID",
  "promotion_id": "promotion UUID",
  "environment": "production",
  "configuration_version": 1,
  "source_sha": "merged source commit SHA",
  "artifact_digest": "sha256:artifact digest",
  "delivery_revision": "delivery PR merge SHA",
  "healthy": true,
  "checks": {"rollout": true, "smoke": true},
  "observed_at": "RFC3339 observation time",
  "nonce": "new UUID per observation"
}
```

The observer must report the exact delivery merge SHA. Reusing a nonce with a changed document, stale/out-of-order evidence, wrong source/artifact/revision or forged signature is rejected. Repeated identical reports are idempotent while the operation is active. Health requires fresh observations spanning the configured window plus elapsed wall time. A missing report cannot become healthy. Configuration changes invalidate earlier health qualification.

## Cancellation and recovery

**Cancel** stops Reforge publication before it starts. An already-created candidate branch can remain for normal native cleanup. Once publication may have occurred, cancellation holds the environment, stops Reforge merge authority and requests normal cancellation of any controlled native merge queue. Close the native PR/MR and **Observe** to confirm cancellation. Native merges may win an in-flight race; canonical observation records that outcome.

Enable recovery in the configuration before using it. Supply the failed promotion ID and a known healthy promotion ID, plus fresh provenance for that known healthy artifact. Recovery creates a new manifest proposal, native approval/merge operation and health window; it preserves the failed promotion. Source/delivery repository, environment and manifest target identities must match. It never rewrites Git history or rolls back through Kubernetes APIs.

## Local acceptance

Run the disposable provider services described by the development setup, then:

```sh
set -a
. .local/development.env
set +a
REFORGE_TEST_ROOT="$PWD" REFORGE_LIVE_GITOPS_TEST=1 go test -race ./test/integration -run '^TestGitOps' -count=1 -timeout=5m
```

The live fixture creates and removes its own Gitea repositories. It uses the actual enrolled private connector, protected branch, independent native approval, merge controller and PostgreSQL state. Health signatures come from fixture observers; external Argo CD/Flux installations and hosted infrastructure remain separate certification actions. GitHub/GitLab GitOps publication uses their implemented forge adapters; live protected promotion certification still needs dedicated accounts and protection profiles.
