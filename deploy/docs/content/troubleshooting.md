# Troubleshooting

## The control plane is not ready

`/readyz` returns `503` when the database is unreachable. Check `REFORGE_DATABASE_URL` and
that the migrator has run with the schema owner URL. `/healthz` only reports that the
process is up.

## Sign-in fails

- In development fixture mode the sign-in button creates a local owner; this requires
  `REFORGE_MODE=development` and a loopback address.
- Otherwise check the OIDC issuer, client ID, secret and redirect URL. The redirect must
  match `REFORGE_PUBLIC_URL`.
- A denied organisation means the session has no membership for that deep link.

## A connection test fails

The error distinguishes authentication, permission, reachability, incompatible protocol
and exhausted quota. For a private endpoint, confirm the enrolled runner is active and the
approved route CIDRs cover the destination. Redirects and DNS changes are rejected by
design.

## A run is blocked

Read the recorded reason. Common causes are a missing or unapproved model route, an
unconfigured or paused budget, unknown usage awaiting reconciliation, or a policy denial.
The run detail links to the governing evidence.

## A merge control is disabled

The gate panel names the missing evidence: unknown protection, a pending review, a stale
head, an untrusted check publisher, or a campaign pause. Refresh the evaluation after the
provider state changes.

## The sandbox is unavailable

The runner reports sandbox unavailability when the host lacks the required isolation
primitive. Do not admit untrusted repositories until it is available; see
[Runners](runners.md).

## Documentation links 404

Help links use `REFORGE_DOCS_URL`. Point it at the running docs container or an internal
mirror. The default is `/docs/`.
