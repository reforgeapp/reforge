# T28a delivery and campaign review — 2026-09-23

Scope: current-tree local PostgreSQL acceptance for V15, V18 and J05. This review records contract-fixture evidence separately from native forge and external delivery certification.

## Current-tree checks

Build revision: `9a39e87`
Go: `go1.27.1 linux/amd64`

Executed from repository root:

```sh
set -a
source .local/development.env
set +a
python3 scripts/check-test-database.py
REFORGE_MIGRATION_DATABASE_URL="$REFORGE_TEST_MIGRATION_DATABASE_URL" go run ./cmd/migrate
go test -race -count=1 ./test/integration -run '^(TestCampaign|TestDeployment|TestGitOps)' -timeout=5m
```

The guard accepted only `reforge_test` on `127.0.0.1:55432`; migrations applied. Result: `ok reforge/test/integration 33.355s` in `.local/t28a-delivery-campaign/pg-acceptance.log`.

Passing selected PostgreSQL scenarios cover:

- Campaign canary failure and unknown outcome stop expansion; fixed membership survives a changed selection; changed connection qualification pins pause further dispatch.
- Campaign progression remains bounded; 1,000-member organisation does not starve a one-member organisation. Repair campaign checks cover budget exhaustion/unknown spend and guarded resume.
- Pipeline campaign calls real deployment and campaign services over PostgreSQL with a contract provider: dispatch/action persistence, signed health completion, lost campaign-action reconciliation without duplicate dispatch, pause cancellation and unknown outcome handling.
- Deployment native completion remains `completed_unverified` until matching signed health passes the observation window. Mismatched run/artifact and invalid signatures fail; failed deployment recovery is a separate preview/request and must also receive health evidence.
- GitOps health rejects forged signatures, wrong source/delivery revisions, wrong artifact and stale evidence; nonce replay survives service reconstruction; unhealthy evidence persists as unhealthy.

The campaign failure/unknown executor and native pipeline/GitOps provider are test contract implementations, not real forge Actions or a reconciler. PostgreSQL persistence and service transitions are real in this run.

## Native provider and GUI limits

`TestGitOpsLiveProtectedPromotion` was not enabled. No Gitea process listens on `127.0.0.1:53000`, no Gitea executable/image is installed, and the test requires `REFORGE_TEST_ROOT` plus disposable Gitea actor tokens. Its source/delivery repository mutations were therefore not attempted. Historical disposable Gitea evidence is recorded separately in `progress.md`; this report does not refresh it against this revision.

`web/tests/deployments.spec.ts` stubs deployment responses for state transitions. Its real-development scenario verifies only an empty deployment list. `web/tests/campaigns-live.spec.ts` drives a real fixture-auth scheduled campaign lifecycle but does not dispatch a delivery. J05 therefore still lacks a current connected GUI flow through native approval, pending delivery, health and separately authorised recovery.

## Remaining qualification

- Run the opt-in GitOps live contract against a fresh pinned disposable Gitea version with isolated PostgreSQL and tokens; retain test-created repositories only within that disposable fixture and verify cleanup.
- Exercise pipeline and GitOps against authorised native providers/reconciler, preserving native approval gates and binding health to exact run, source, artifact, environment and delivery revision.
- Complete connected GUI J05 state transitions with backend-backed delivery records. Keep pipeline completion visibly unverified until health is accepted.
- Run actual campaign dispatch with native delivery, canary failure/unknown health, changed pins, restart and multi-tenant contention. Current campaign scheduler/fairness evidence uses a simulated executor; pipeline contract checks use a provider contract fixture.

No new regression test was added: current integration tests already exercise the local contract boundaries above. No external repository, customer infrastructure, or Kubernetes API was touched. G5 remains open.
