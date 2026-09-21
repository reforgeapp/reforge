# Reforge documentation

Reforge is an Apache-2.0 repository maintenance service. It imports repositories from
GitHub, GitLab and Gitea, discovers maintenance work, runs bounded repairs on an isolated
runner, publishes attributable changes, and gates merge and delivery through policy and
native provider controls.

This site covers the workflows that are implemented in the shipped source. Capabilities
that are implemented but not yet certified against a live provider are marked as such in
the [support matrix](support-matrix.md). No page promises a provider capability that the
product cannot verify.

## Editions

- **Self-hosted** runs from the Compose stack in this repository. It uses the same source,
  schema, API and interface as the hosted edition.
- **Hosted** runs the same binaries with GitOps-managed infrastructure. Tenant isolation,
  infrastructure provisioning and billing are operator concerns and are not configured
  from the browser.

## Where to start

1. [Install](install.md) the stack.
2. Connect a forge in [Connections](connections.md).
3. Enrol a [runner](runners.md) if repositories or models are private.
4. Import [repositories](repositories.md) and review [findings](findings.md).
5. Read [Runs and repair](repair.md) before enabling automatic publication.
6. Read [Changes and merge](merge.md) and [Deployments](delivery.md) before enabling
   protected merge or delivery.

## Getting help in the product

Every route has an on-demand **Help** control that opens the matching topic. Help links
honour `REFORGE_DOCS_URL`, so a deployment can point them at this versioned site or at an
internal mirror.
