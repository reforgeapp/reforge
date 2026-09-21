# Install

Reforge ships as containers. A clean install does not require Go, Node or Python on the
host, and it does not depend on repository-specific `/tmp` paths.

## Host prerequisites

- A container runtime with Compose (Podman or Docker).
- Two CPU cores and 4 GB RAM for a small evaluation; production sizing depends on
  repository count and concurrent runs.
- A PostgreSQL 18 database reachable from the control-plane container.
- An encryption key for credentials, base64-encoded, generated on the host and stored
  outside the image. The KMS backend is used when configured; otherwise envelope
  encryption uses this key.

!!! warning "Containers are not tenant isolation"
    Running Reforge in containers does not by itself isolate hostile repository code.
    Untrusted execution requires the isolated runner with a sandbox runtime. Do not
    connect untrusted repositories until [Runners](runners.md) reports the sandbox as
    available.

## Compose

1. Copy `deploy/compose/.env.example` to `.env` and set at minimum:
   - `REFORGE_DATABASE_URL` — runtime database role, not the migration owner.
   - `REFORGE_MIGRATION_DATABASE_URL` — schema owner, used only by the migrator.
   - `REFORGE_ENCRYPTION_KEY` — base64 key material.
   - `REFORGE_PUBLIC_URL` — the browser-visible origin.
2. Run the migrator once, then start the stack:

   ```sh
   docker compose --env-file .env -f deploy/compose/compose.yaml run --rm migrator
   docker compose --env-file .env -f deploy/compose/compose.yaml up -d
   ```

3. Open `REFORGE_PUBLIC_URL`. In development fixture mode the sign-in button creates a
   local owner. In every other mode you must complete OIDC or the self-hosted one-time
   bootstrap described below.

## First administrator

Self-hosted installs create the first organisation through the one-time bootstrap:

1. The operator sets `REFORGE_BOOTSTRAP_TOKEN` and `REFORGE_BOOTSTRAP_EXPIRES_AT` for the
   first start.
2. Sign in through the configured identity provider, then open `Organisation` and enter
   the bootstrap token to create the initial organisation and owner membership.
3. The token is single-use. Remove it from the environment and restart after use.

The host OIDC issuer, client ID and client secret are configured by the operator; the
browser cannot create or change infrastructure authentication.

## Editions and configuration

`REFORGE_EDITION` is `self-hosted` or `hosted`. Both use the same schema and interface.

Development fixture authentication requires explicit development mode and a loopback
listen and public address, so it only works when the process runs directly on the host
loopback; it is not usable through container port mapping. A container deployment
therefore needs an HTTPS `REFORGE_PUBLIC_URL`, OIDC, or the one-time self-hosted
bootstrap token described above. The Compose file passes `REFORGE_MODE` and
`REFORGE_FIXTURE_AUTH` through only for host-loopback development.

See [Security model](security.md) for credential custody, egress rules and the runner
trust boundary, and [Support matrix and limitations](support-matrix.md) for what is
certified in this build.
