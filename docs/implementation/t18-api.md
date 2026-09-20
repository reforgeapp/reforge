# T18 discovery API

Discovery routes expose organization-scoped findings, advisory imports, repository maintenance configuration, and repository scan status/actions.

All routes use the identity session. Findings and maintenance configuration mutations require JSON plus quoted `If-Match` versions. Successful versioned responses return quoted integer `ETag` values. Scan start returns `202`.

Finding filters: `q`, `repository_id`, `state`, `category`, and `severity`. List routes use standard `limit` and `cursor` pagination.

Stale evidence, overlapping maintenance work, and incomplete or unreviewed evidence return actionable `409` responses. Other authorization and identity failures use shared identity error handling.
