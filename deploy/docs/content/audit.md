# Audit

The **Audit** route is an immutable action history. Rows record the actor, action, object,
policy version and request identity, and link to the provider action where one exists.

Filters cover actor, action, repository, time range and object. The export produces the
loaded page as NDJSON and respects the same permissions and retention as the on-screen
list.

Audit is scoped: an owner or administrator can read organisation-wide events; other roles
see events for the repositories they can access. Session revocation and repository access
removal take effect immediately for new reads.

Reforge records the authority that admitted an action, not an impersonation. Application
approvals are labelled distinctly from provider reviews, and a recorded approval never
pretends to be a native one.
