# T31 integrated local acceptance — 2026-09-23

`TestCustomProfilePostgresGVisorWorkflow` joins durable PostgreSQL profile approval,
repair enqueue, runner enrollment/claim, dispatcher authorization/report, actual local
custom-command execution in gVisor, frozen baseline/candidate/target checks, and budget
reservation read-back. The command is an approved fixed Node argv. Unknown usage remains
held. Publication is exercised through a fixture endpoint that requires validated report,
staged candidate, matching plan/head and passing native-check records; it does not certify a
real forge publish.

`TestCustomProfileConcurrentAuthorizeHonorsProfileLimit` dispatches two different jobs for
the same approved profile from separate runner leases and repositories concurrently. The
profile row lock serializes approval revalidation and the active-run count. PostgreSQL records
one running run; other attempt receives `ErrCapacity`.

`TestCustomProfileDispatcherTerminalReplay` records a terminal report, repeats authorization
for same lease, and verifies dispatch fails and persisted terminal state remains unchanged.
The failed reservation transition rolls back the upsert; no duplicate execution was observed.

Checks passed with race detector against disposable local PostgreSQL 18.6 and local gVisor:

```text
REFORGE_CUSTOM_PROFILE_E2E_TEST=1 REFORGE_TEST_DATABASE_URL=<disposable runtime URL> REFORGE_TEST_MIGRATION_DATABASE_URL=<disposable migration URL> go test -race -v ./test/integration -run '^TestCustomProfile(PostgresGVisorWorkflow|DispatcherTerminalReplay|ConcurrentAuthorizeHonorsProfileLimit)$' -count=1
```

Output: `.local/t31-integrated/test.log`. Focused default-path compile/skip check also passed;
the opt-in tests skip unless `REFORGE_CUSTOM_PROFILE_E2E_TEST=1` and both disposable database
URLs are supplied. No live forge or model provider is contacted.

External qualification remains open: official subscription entitlement, permitted route,
headless and approval-interception behaviour, supported runtime/version matrix, hosted
isolation and protected native forge publication. G5 remains open.
