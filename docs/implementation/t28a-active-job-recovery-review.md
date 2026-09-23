# T28a active-job recovery review

Observed 2026-09-23 using `.local/development.env` and `scripts/check-test-database.py`. It is the shared local PostgreSQL service, targeting the explicitly guarded disposable `reforge_test` database. Test records use fresh UUID organisations; runner expiry update is constrained to that test organisation and job. No database reset or truncation. No customer or production data.

Command:

```sh
set -a; . .local/development.env; set +a
python3 scripts/check-test-database.py
go test -race -count=1 ./test/integration -run '^(TestActiveJobLeaseHolderProcess|TestActiveJobRecoveryAfterWorkerProcessKill|TestRunnerLeaseHolderProcess|TestRunnerLeaseRecoveryAfterSupervisorProcessKill)$'
```

Result after subprocess timeout/cleanup change: `ok reforge/test/integration 4.578s`.

The test starts a separate Go test process, has it claim a workflow lease, then kills it. After the one-second lease expires naturally, a new workflow service recovers and reclaims the same job and operation with a new attempt and higher fence. Heartbeat and completion using the killed process lease are rejected; replacement heartbeat succeeds; prior attempt is recorded as lost.

A second subprocess uses a real enrolled runner supervisor to claim a pool-scoped assignment, then is killed. The test expires that lease directly in the disposable database, recreates workflow and runner service objects, and reclaims the same operation with a higher fence and new attempt. Old job credential heartbeat is rejected; replacement credential heartbeat succeeds.

Both child processes use 15-second `CommandContext` deadlines and idempotent cleanup that kills and waits once on every early exit. This exercises OS process termination, persisted PostgreSQL state, service-object recreation, recovery and fencing. It does not restart a Docker/Compose container or the full HTTP server. Runner assignment uses fixed one-minute TTL, so test advances expiry in disposable PostgreSQL rather than waiting a minute. It does not exercise real gVisor work interruption, external provider outcome reconciliation, host resource isolation, or deployed runner rotation. Existing Compose restart evidence is process-only installation evidence and does not combine restart with an active job. T28a/J09/V19 remain partial; external deployment and host qualification remain open.
