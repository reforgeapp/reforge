# Deployments

The **Deployments** route has two delivery modes: native pipelines and GitOps
promotions.

## Native pipelines

Deployment rows link the repository, merged revision, immutable artifact, workflow,
environment, native approval, rollout and verification status. **Pipeline succeeded** is
shown separately from **application verified**. A workflow without signed health evidence
is reported as “Completed; health verification unavailable”.

Requesting a deployment selects an allowlisted workflow and environment with a
server-resolved artifact; there is no arbitrary shell field. Production approval stays in
the native delivery system. Observation-only mode imports a native run without taking
cancellation authority.

## GitOps

A GitOps configuration names a delivery repository, a manifest field and an immutable
image digest. Reforge proposes a deterministic change and gates it through the same
protected merge path; the existing reconciler performs the rollout. Reforge never mutates
a cluster directly.

## Recovery

**Request recovery** names the pre-approved rollback pipeline or GitOps revert, the target
artifact and the required approvers. A database migration that requires manual recovery
does not get an automatic rollback control. Recovery is a distinct recorded operation.
