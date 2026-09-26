# Install

Reforge ships as containers. A clean install needs Compose and a container runtime on the
host; Go, Node, Python, database clients and repository-specific `/tmp` paths are not
required.

## Host prerequisites

- Docker Compose or compatible Compose implementation.
- Two CPU cores and 4 GB RAM for a small evaluation; size production from repository count
  and concurrent runs.
- HTTPS reverse proxy and DNS name for the browser-visible origin.
- Linux with cgroup v2 and permission to run a privileged container, for the built-in
  runner.
- PostgreSQL 18 container storage with backups for the `pgdata` and `artifacts` volumes.
- Secret storage for database credentials, the OIDC client secret and the base64-encoded
  credential encryption key.

!!! warning "Containers are not tenant isolation"
    Running Reforge in containers does not by itself isolate hostile repository code.
    Untrusted execution requires the isolated runner with a sandbox runtime. Do not
    connect untrusted repositories until [Runners](runners.md) reports the sandbox as
    available.

## Compose

1. Copy `deploy/compose/.env.example` to `.env`. Replace every `REPLACE_` value. Generate
   URL-safe database secrets with `openssl rand -hex 32`; copy each migration/runtime
   password into its matching database URL. `POSTGRES_PASSWORD` is the PostgreSQL
   bootstrap/admin credential; the Reforge server does not use it. Keep `.env` outside
   source control. Generate the credential key with
   `openssl rand -base64 32` and store it outside the image.
2. Set `REFORGE_PUBLIC_URL` to the HTTPS origin used by the browser. Configure the reverse
   proxy certificate and route application paths to `REFORGE_BIND` (default
   `127.0.0.1:8080`). Route `/docs/` and its assets, search and version paths to
   `REFORGE_DOCS_BIND` (default `127.0.0.1:8082`); keep `REFORGE_DOCS_URL=/docs/`.
   Configure the OIDC application with redirect URI
   `${REFORGE_PUBLIC_URL}/auth/callback`; set the issuer URL, client ID and client
   secret in `.env`.
3. Start the stack:

   ```sh
   docker compose --env-file .env -f deploy/compose/compose.yaml up --build -d
   ```

   Compose creates or updates the non-superuser migration and runtime roles, runs schema
   migrations, applies runtime grants, then starts the server and the
   [built-in runner](runners.md#built-in-runner). Both role passwords must be
   non-empty and match their URLs. The database service account is used only by the one-shot
   role bootstrap; Reforge server connects as `reforge_runtime`.
4. Open `REFORGE_PUBLIC_URL`, sign in through OIDC and use the self-hosted one-time
   bootstrap form to create the first organisation and owner membership.

On an existing volume, the bootstrap preserves database contents and never revokes role
memberships; configured role passwords are reconciled from `.env`. It refuses to continue
if either Reforge role is a member of another role, so review and remove such memberships
explicitly before startup. It transfers database ownership only when the database is
currently owned by `POSTGRES_USER` or `reforge_migrator`. For a custom-owned database,
arrange ownership for `reforge_migrator` before starting Compose; the bootstrap stops
without changing ownership otherwise. PostgreSQL does not apply
`POSTGRES_PASSWORD` changes to an existing volume. Back up the database and encryption key
before upgrades. Do not remove the `pgdata` volume as an upgrade step.

## First administrator

Set a random `REFORGE_BOOTSTRAP_TOKEN` of at least 32 characters and an
`REFORGE_BOOTSTRAP_EXPIRES_AT` timestamp in RFC3339 format before the first sign-in. A
signed-in user without an organisation enters the token and organisation name to create the
initial organisation and owner membership. Token is single-use; remove it from `.env` and
restart after use.

## Editions and configuration

`REFORGE_EDITION` is `self-hosted` or `hosted`. Both use the same schema and interface.
Development fixture authentication requires explicit development mode and a loopback
listen and public address. It is not usable through container port mapping. Production
Compose therefore requires HTTPS `REFORGE_PUBLIC_URL` and configured OIDC for sign-in;
the one-time bootstrap token creates the first organisation after authentication and does
not replace OIDC.

See [Security model](security.md) for credential custody, egress rules and the runner trust
boundary, and [Support matrix and limitations](support-matrix.md) for certification state.

## Local non-development verification

`make install-check` (`scripts/install-check.sh`) runs non-development mode against a local
OIDC issuer and CA/TLS proxy. It creates and migrates a scratch database, checks readiness,
production metadata, OIDC redirect and authorization-code login, then optionally repeats
the login in a browser. It needs a maintenance database URL, migration and runtime URLs,
the operator encryption key and PostgreSQL client binaries. This verification harness uses
host Go; supported installation uses containers. It proves local production-mode startup
and OIDC login, not customer OIDC or hosted cluster certification.

## Hosted GitOps reference

`deploy/gitops` is a versioned reference for deploying the control plane from a GitOps
repository rather than mutating a cluster directly. The base renders a namespace, migration
Job, control-plane Deployment and Service, and placeholder Secret. The `overlays/example`
overlay pins the published image. Replace Secret placeholders from your secret manager,
keep the migration Job before the Deployment, and let your existing reconciler apply the
rendered output.

The reference does not run Reforge itself as a reconciler and does not certify a hosted
cluster: you still need PostgreSQL, an OIDC issuer, HTTPS origin, customer-owned runner host
and sandbox prerequisites described in [Security model](security.md). No API or task
container receives a Docker socket and no credentials are baked into images.
