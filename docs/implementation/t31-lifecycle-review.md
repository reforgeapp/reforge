# T31 local lifecycle evidence

Reviewed 2026-09-23 against [T31 contract](t31-custom-runtime.md) and the acceptance clause in [backlog](backlog.md#t31--administrator-approved-custom-command-runtime-profiles).

## Checks run

All focused checks passed:

- `go test ./internal/customcmd ./internal/runnerclient ./internal/maintenance/repair` — domain, executor, patch extraction and repair validation unit suites. Log: `.local/t31-lifecycle/unit.log`.
- `REFORGE_TEST_DOCKER=1 go test -count=1 -run TestRealContainerProfileProtocol -v ./internal/customcmd` — real local Docker protocol cases for input/output with unknown usage, malformed event, nonzero exit, wall budget, cancellation and secret isolation. Log: `.local/t31-lifecycle/docker-protocol.log`.
- With `.local/development.env` sourced, `go test -count=1 -v -run 'TestCustomProfile(DispatchAuthorizesExecutesAndFencesRevocation|ApprovalRevocationAndBind|ValidationAndAutomation|TenantIsolation|ExpiredApprovalDoesNotBind)$' ./test/integration` — all five PostgreSQL integration tests passed against fixture-enforced disposable `reforge_test`. Log: `.local/t31-lifecycle/postgres-integration.log`.
- `REFORGE_CUSTOM_PROFILE_PROCESSOR_TEST=1 go test -count=1 -v -run '^TestCustomProfileRealProcessorGVisor$' ./test/integration` — real `RepairProcessor` + gVisor, local generated JavaScript rootfs, valid and rejected candidate cases. Log: `.local/t31-lifecycle/processor-gvisor.log`.

The disposable DB helper rejects any database URL whose path is not `/reforge_test`. No customer forge, paid API, or official runtime was used.

## What evidence establishes

| Lifecycle stage | Existing evidence | Limit |
|---|---|---|
| Create, draft state, owner approval, version bump, stale approval conflict, immutable digest/version returned by bind, revoke and future bind rejection | `TestCustomProfileApprovalRevocationAndBind` against PostgreSQL | It does not attempt to mutate a persisted run binding. |
| Tenant isolation; automation cannot manage profiles; invalid tag digest and shell metacharacters rejected; expired approval is not considered valid | `TestCustomProfileTenantIsolation`, `TestCustomProfileValidationAndAutomation`, `TestCustomProfileExpiredApprovalDoesNotBind` | These cover service/API domain boundary, not browser flow. |
| Policy/route setup, repair preview binding, enqueue, runner claim/progress and server authorization; durable run row created; profile revoke fences a later authorize | `TestCustomProfileDispatchAuthorizesExecutesAndFencesRevocation` | It calls `Dispatcher.Report` directly with hand-authored `completed_unverified`, unknown usage and empty events. It does not run `runnerclient.RepairProcessor`, the Docker executor, extraction, validation or publication. The name overstates execution coverage. |
| Typed final-result protocol, process limits, timeout/cancel, secret isolation | `TestRealContainerProfileProtocol`, plus executor unit cases | Exercises container protocol/executor, not server-to-runner lifecycle. |
| Changed files become source-only patches | `TestCustomProfilePatchExtraction`; `TestCustomProfileRealProcessorGVisor/valid_candidate` | Unit test uses an in-memory runtime; opt-in processor test executes a fixed-argv Node command in gVisor, reports typed success, then checks extracted source patch. |
| Frozen baseline/candidate/target validation, protected path rejection and empty patch rejection | `TestValidateCustomRequiresBaselineAndTarget`; `TestCustomProfileRealProcessorGVisor` | Processor test reaches all three frozen checks in the real sandbox; fixture gates stage and publish on the fixture-recorded validated report; publish also checks successful native result evidence in request body. `/repair/native-checks` is a separate failure-evidence route, gated on validated report and stage, and is not expected on successful native validation. |
| Native publication authority and rejected invalid/unverified candidates | `test/integration/repair_test.go` publication scenarios; `TestCustomProfileRealProcessorGVisor/failing_candidate_denied` | Processor test proves failing custom candidate returns before stage/native-check/publication. Its success publish handler verifies fixture-recorded report state, stage, candidate SHA, plan digest and passing native result payload, but does not prove database or forge authority. |
| Budget reservation creation and settlement/unknown paths | Dispatch code reserves and marks dispatched; report code settles known usage or marks unknown. The custom dispatch integration installs a one-request route and org limit, then reports unknown usage. | Current integration does not inspect reservation state, prove cap denial, known-usage settlement, unknown reservation hold, or retry/idempotency behavior. |

## Remaining local acceptance gap

`TestCustomProfileRealProcessorGVisor` now connects the actual runner `RepairProcessor` to a real gVisor command and frozen validation. Its valid case observes stage then publish after fixture-recorded validated report, with passing native result evidence in the publish body; its invalid case proves stage, native-check and publish routes are not reached. The native-check endpoint remains a fixture handler for failure evidence and performs no provider check; the publish fixture verifies processor-supplied native result payload but performs no forge write. All control-plane endpoints are in-memory HTTP fixtures, so this does not establish persisted profile/run/version/digest/reservation, budget settlement/hold/cap enforcement, revocation races, or native forge publication authority in the same lifecycle. The earlier PostgreSQL dispatch test still calls `Dispatcher.Report` directly and has those coverage limits.

Close remaining local gap with a disposable-PostgreSQL runner test that wires actual controller endpoints and budget store around this processor path, then exercise known/unknown usage, cap denial, revoke/fencing and persisted immutable binding. Keep native forge publication under existing protected-authority integration tests or add a local Gitea flow when prerequisites are available. This test used development gVisor mode with no delegated cgroup subtree; it exercises gVisor command isolation but does not certify memory/CPU/PID cgroup enforcement or hosted tenant isolation.

## Certification boundary

Local Docker and PostgreSQL evidence does not certify official subscription runtimes, entitlement, headless behavior, approval interception, hosted cgroup isolation or production topology. T31 remains disabled until each supported runtime/configuration has documented permitted-route evidence and independent certification. T31a typed-result fix is locally exercised by the real container protocol test; its external runtime qualification remains open.

Confidence: 90% that report maps local T31 lifecycle evidence and gaps accurately. The custom processor-to-validation path passed; full persisted dispatch, budget, revocation and native forge publication acceptance remains open.
