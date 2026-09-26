# Security model

## Tenancy

Every operational table is scoped by organisation and protected by PostgreSQL row-level
security. The API resolves a live actor from the session on each request; repository and
team membership is re-read from the database rather than trusted from the client.
Revoking a session, membership or repository access takes effect on the next request and
on long-lived event streams.

## Credentials

Connection secrets are write-only from the browser. They are encrypted with an envelope
key (local key or KMS) and are never returned in API responses, events or logs. Rotation
creates a new credential version and revocation is immediate. The interface never stores
credential values in browser storage.

Do not place credentials in images. The Compose stack injects secrets from the host
environment or an operator-managed secret store.

## Runner trust boundary

Repository code, model calls and forge operations run on a runner outside the control
plane. The runner:

- enrols with a short-lived, single-use instruction rather than a permanent token file;
- receives per-job credentials that are scoped to the job and expire;
- executes commands inside a sandbox with no network by default, resource limits and
  cleanup of process groups;
- cannot upload artifacts or publish results after its lease is fenced.

The built-in runner enrols with a token generated on first start and shared with the server
through the `builtin` volume. It runs privileged on the control-plane host, so gVisor
is the only boundary between repository code and the database and encryption key. Use
an enrolled runner on a separate host for untrusted repositories.

Private network routes are fixed-operation and pinned to an enrolled runner and explicit
CIDRs. Redirects, DNS rebinding and metadata destinations are rejected.

!!! warning
    The control plane does not give API or task containers a container socket. If your
    deployment mounts one, the isolation guarantees above do not apply.

## Supply chain

CI fails on HIGH or CRITICAL vulnerabilities with an available fix, in dependencies and in
each published image, and on verified or unverifiable secrets in the git history. The
runner's gVisor binaries are upstream releases pinned by SHA-512; their embedded Go runtime
is patched by upgrading the pinned release, so the image scan excludes `/app/gvisor`.

## Subscription and provider terms

Reforge never harvests CLI OAuth caches, never treats a successful login as entitlement,
and never falls back to a paid API route without an explicit, budgeted decision. Agent
routes stay disabled until the exact runtime, account custody, topology and terms are
qualified. See [Agent runtimes](agents.md).

## Kubernetes

Reforge reads and proposes Kubernetes changes through GitOps repositories only. It does
not mutate clusters directly. Native provider approvals remain in the provider.
