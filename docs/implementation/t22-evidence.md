# T22 native evidence contract

This is an operator contract for current Reforge code. It describes signing and read-only observation; it does not grant native approval, merge, deployment, or GitOps authority.

## Offline signing

Generate an owner-only Ed25519 key. `keygen` refuses an existing path; keep private key outside the repository with mode `0600`:

```sh
go run ./cmd/evidence keygen --key-file /secure/reforge-evidence.key > /secure/reforge-evidence.pub
```

The signer accepts one JSON document, rejects unknown fields and documents larger than 1 MiB, and prints:

```json
{"document":{},"signature":"base64-ed25519"}
```

Sign each kind with its exact current Go shape:

```sh
go run ./cmd/evidence sign --key-file /secure/reforge-evidence.key --kind provenance --document provenance.json
go run ./cmd/evidence sign --key-file /secure/reforge-evidence.key --kind native-health --document deployment-health.json
go run ./cmd/evidence sign --key-file /secure/reforge-evidence.key --kind gitops-health --document promotion-health.json
```

`provenance.json` fields are `org_id`, `repository_id`, `source_sha`, `artifact_digest`, `build_id`, `issued_at`, and `expires_at`. The configured deployment public key verifies this document before promotion.

`native-health` fields are `org_id`, `deployment_id`, `environment`, `configuration_version`, `source_sha`, `artifact_digest`, `run_id`, `run_attempt`, `revision`, `healthy`, `checks`, `observed_at`, and `nonce`. POST the signed document's `document` to `/api/v1/deployment-health/{orgID}/{deploymentID}` with its `signature` in `X-Reforge-Signature`.

`gitops-health` fields are `org_id`, `promotion_id`, `environment`, `configuration_version`, `source_sha`, `artifact_digest`, `delivery_revision`, `healthy`, `checks`, `observed_at`, and `nonce`. The GitOps health callback is not a native deployment approval and must remain attributed to the configured delivery revision.

Health evidence must identify the exact source, immutable artifact, run identity, configuration version, revision, observation time, and configured checks. The reporter must be an authenticated operator or deployer that can prove those bindings; a status page, unverified webhook, or manually copied result is insufficient. Reports outside the configured freshness/window, with a wrong run attempt, or with a different artifact/source remain unknown or failed.

## Observe-only deployment tracking

Configure the environment with `mode: "observe"`, enabled, workflow identity, public keys and health checks. Tracking does not dispatch a workflow, approve an environment, cancel a run, merge a change, or mutate a native system. The existing native run must already exist and be readable.

POST `/api/v1/orgs/{orgID}/deployment-configurations/{environment}/track` with CSRF and identity JSON. `deployment.TrackRequest` embeds `PreviewRequest`, so fields are flat:

```json
{
  "change_id":"123",
  "source_sha":"40-hex-source",
  "artifact_digest":"sha256:64-hex-digest",
  "provenance":{"document":{"org_id":"…","repository_id":"…","source_sha":"…","artifact_digest":"…","build_id":"…","issued_at":"…","expires_at":"…"},"signature":"…"},
  "run_id":"456",
  "idempotency_key":"track-unique-key"
}
```

The service checks the merged change SHA, observe-only configuration, repository/connection versions, signed provenance, canonical run identity, and idempotency request. Success returns `201` and a deployment operation. The operation starts as observed native state; it becomes healthy only after authenticated health reports satisfy the configured observation window. Repeating the same idempotency key with different content conflicts.

## Native provider prerequisites

GitHub/GitLab workflow mode requires separately certified provider connection metadata, immutable workflow/configuration identity, exact source/ref, artifact provenance, native protected-environment rules, and current native approval/check evidence. External certificates or platform plan claims must be stored as qualification evidence with provider, server version, connection version, reference, SHA-256, dates, and all four qualification flags. Gitea native Actions delivery remains disabled; T23 provides its protected GitOps path. Uncertified external deployers never gain mutation authority from status reporting.

Do not treat Reforge policy approval, a successful trigger response, deployment tracking, or signed health alone as native approval authority. Unknown, stale, missing, or conflicting provider evidence blocks progression and requires reconciliation.

Write signed health, then send the exact document and signature:

```sh
go run ./cmd/evidence sign --key-file /secure/reforge-evidence.key --kind native-health --document deployment-health.json > signed-health.json
jq .document signed-health.json > health-body.json
curl --fail --request POST --header 'Content-Type: application/json' \
  --header "X-Reforge-Signature: $(jq -r .signature signed-health.json)" \
  --data-binary @health-body.json \
  "$REFORGE_URL/api/v1/deployment-health/$ORG_ID/$DEPLOYMENT_ID"
```

The reporter derives revision/digest/checks from the deployed workload and authenticated build/reconciler metadata. Protect its private key separately from the coding runner. `healthy: true` is accepted only with the correct signature, identity, freshness, criteria and elapsed observation window; the signer itself does not verify a workload.
