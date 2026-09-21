# Teams and membership

The **Organisation** route administers teams, membership and repository scope. Owner or
administrator access is required; listing and editing members requires the owner role.

## Teams

Create a team, rename it, and select the repositories it contains. Each change uses the
team's current version, so a stale edit is rejected instead of overwriting another change.

## Membership

Members are listed by user. An owner can change a member's role and repository access, or
remove the member. Removing the last owner is rejected. Role changes are version-checked
and audited.

## Identity and retention

The OIDC issuer, session lifetime and audit retention are configured by the operator at
install time. The browser cannot change infrastructure authentication. See
[Install](install.md) and [Security model](security.md).

Repository access can be granted through a team or directly to a member. A member with
`all_repositories` bypasses per-repository grants and should be reserved for owners and
administrators.
