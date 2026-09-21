# Connections

**Connections** has three groups: forges, models and agents, and delivery integrations.

## Forges

- GitHub uses an App installation or a token connection.
- GitLab uses a token; self-managed instances set the endpoint and CA.
- Gitea uses a token and supports scoped identity and OpenAPI capability discovery.

After creating a connection, run the capability probe. The probe records the server
version, scopes and the features that are actually available. A missing feature disables
only the affected action and shows the reason.

## Credentials

Credentials are write-only. The interface shows a fingerprint or last characters where
the provider is safe, the rotation date and a revoke control. Rotation and revocation are
immediate. Connection failures distinguish authentication, permission, reachability,
incompatible protocol and exhausted quota.

## Private routes

A private forge or model endpoint is reachable only through an approved route: a fixed
host and CIDR set pinned to an enrolled runner. Redirects, DNS changes and metadata
destinations cannot expand the scope.

## Delivery

Delivery connections configure native pipeline access or a GitOps delivery repository.
See [Deployments](delivery.md).
