# T27 isolation, concurrency and recovery qualification

Status: local qualification run recorded 2026-09-22. Hosted cgroup-enforced isolation and
live-provider certification remain external.

## Build identity

- HEAD `96aa210` (Go 1.27, PostgreSQL 18.6, Node 26, Chromium via Playwright 1.63).
- Disposable local PostgreSQL on `127.0.0.1:55432`; pinned gVisor `runsc` under
  `/tmp/reforge-gvisor/bin`.

## Scenario corpus and results

| Scenario | Command / harness | Result |
| --- | --- | --- |
| Cross-tenant/team isolation, RLS under pooling, lease/queue/outbox recovery, budget contention, webhook replay/rotation, revoked identities, repair/publication reconciliation, merge recovery, custom dispatch | `go test -race -count=1 -timeout 25m ./test/integration/...` | pass `117.416s` |
| Hostile repository corpus: path traversal, symlink escape, truncated/corrupt/duplicate fetch, cancellation, development boundary | `make sandbox-test` (real rootless gVisor) | pass |
| 1,000-member campaign fairness and 100 competing runner claims | integration suite | pass |
| Control-plane scale: 10,000-repo asynchronous import, scoped pagination/search, 50 concurrent sessions, 100 queued claims | `REFORGE_LOAD=1 go test -run TestControlPlaneLoadTargets ./test/integration/` | pass; scan `2.6s`, import `9.8s`, page/search `5ms`, sessions p50/p95/max `16/19/20ms`, claims `2.0s` |
| Encrypted backup restore and key recovery | `scripts/restore-drill.sh` | pass; `tables=73 migrations=33` and key-recovery test |
| Non-development install, migrations, OIDC login, restart | `scripts/install-check.sh` | pass; production `/api/v1/meta`, OIDC login and session verified |
| Container packaging: control, runner, docs images; no Docker socket; non-root | `docker build` + control smoke | pass; uid 10001, `/readyz` 200, no `/var/run/docker.sock` |

No failing scenario was recorded in this run. Zero policy bypasses were observed in the
integration suite.

## External gaps

- Hosted untrusted execution needs a sandbox host with working cgroup delegation; this
  host runs the rootless development boundary only.
- Live forge, paid model and official-agent account certification, and a customer OIDC
  issuer plus hosted cluster, are not supplied.
- The 50-session load target is exercised as concurrent authenticated HTTP sessions, not
  50 real browser contexts.
