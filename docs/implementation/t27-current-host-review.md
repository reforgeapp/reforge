# T27 current host review

Status: local development-boundary checks passed on 2026-09-23. Hosted untrusted
execution, cgroup enforcement and 100 concurrent executions remain unqualified.

## Identity

- Repository tree at run: `cc68e29f928bbc7545a208ceacc0e6a0aacc37f1`.
- Host: Linux 6.18.33.2 WSL2, x86_64; Go `go1.27.1`.
- gVisor: `release-20260914.0`, `/tmp/reforge-gvisor/bin/runsc`, SHA-256
  `c0f4ec0ac1198975d5cf919a78f2302426de096f69eebd33e50125c3ca42d699`.
- Sandbox helper: `bin/reforge-sandbox-tool`, SHA-256
  `799bf8c843111150ebdabefc5b9349643db6ff999e93262a6825fd15908335b2`.
- Probe: `/tmp/reforge-sandbox-probe`, SHA-256
  `1a4e2ce2ee2b3aff3da59219632f2bfc0709d1aa5e5057b17f7a6522cc1589cc`.
- Ephemeral fixture image assembled by `TestRealGVisorBoundaryAndCancellation`: `sha256:5171825f2ed58c7eb2bb3506a7b8a849a10517bf7a9a26b6b86720ed92b55e34`.
- PostgreSQL commands loaded ignored `.local/development.env` with `set -a; . .local/development.env` and exported its `REFORGE_TEST_DATABASE_URL`; the integration command also exported `REFORGE_TEST_MIGRATION_DATABASE_URL`. The harness requires database path `/reforge_test`. Server reported PostgreSQL `18.6`; existing service on `127.0.0.1:55432` was left running.

## Checks

| Surface | Command / cases | Result |
| --- | --- | --- |
| Real rootless gVisor | `REFORGE_TEST_RUNSC=/tmp/reforge-gvisor/bin/runsc REFORGE_TEST_SANDBOX_TOOL=$PWD/bin/reforge-sandbox-tool REFORGE_TEST_SANDBOX_PROBE=/tmp/reforge-sandbox-probe go test -race ./internal/sandbox -run '^TestRealGVisorBoundaryAndCancellation$' -count=1 -v` | Pass, 8.256s. Existing test covers untrusted-development rejection, absent production-cgroup rejection, snapshot/image integrity, readonly mounts, cross-workspace separation, traversal/symlink/FIFO rejection, bounded disk/output, timeout descendant termination, caller cancellation, orphan recovery and shutdown cleanup recovery. |
| PostgreSQL identity, event and workflow | `go test -race -count=1 -timeout 12m ./test/integration -run '^(TestRuntimeIsolationAndPooledContext|TestEventStreamScopesAndLiveRevocation|TestConnectionPersistenceRotationAndRevocation|TestConnectionQueuedProbeCannotUseRevokedAuthority|TestModelBrokerPrivateOllamaBudgetAndReplay|TestCustomProfileTenantIsolation|TestRepairFrozenPreviewArtifactBindingAndLostPublication|TestTaskBudgetHTTPAndCurrentPolicyDispatch)$' -v` | 7 pass, 1 skip in 21.043s. Skip: private Ollama fixture not explicitly configured. Covers pooled RLS context, repository-scoped SSE and immediate session revocation, cross-tenant profile lookup, write-only rotated/revoked connection credentials and queued probe revocation, budget/policy dispatch fencing, repair validation/artifact evidence and cancellation. |
| PostgreSQL runner/artifact fencing | `go test -race -count=1 ./internal/runner -run '^(TestJobCredentialsFilterBeforeClaimAndRevokeFences|TestArtifactsRefreshMembershipAndJobScope|TestPrivateOperationRevocationSerializesWithInvocation|TestCancellationAndPoolGrantChange|TestJobMethodExpiryAndPolicyRevocation)$' -v` | 5 pass, 2.053s. Artifact streaming stops after membership revocation and expired job lease; revoked runner/pool grants fence operations. |
| PostgreSQL budget | `go test -race -count=1 ./internal/budget -run '^(TestHundredContendingHierarchicalReservations|TestUnknownRolloverSettlementAndDebt|TestBudgetRevalidationAndUnknownRoutes|TestInjectedAuthorityChecksAndRetainedSpend)$' -v` | 4 pass, 2.272s, including 100 concurrent reservations and unknown-usage accounting. |
| Private route / endpoint | `go test -race -count=1 ./internal/privateconnector ./internal/network -run '^(TestRevocationAfterReadinessPreventsCredentialDelivery|TestTimeoutAndServerLossAreUncertainWithoutReplay|TestCrossTenantConnectionAndExtraArgumentsDenied|TestGrantLivenessEndsOnCancellationAndRejectsOtherCapability|TestEndpointAndPrivateRouteBoundaries|TestDNSRebindingAndLiteralDial|TestRequestOriginPathAndMethodGuard|TestPrivateDNSCIDRAndRouteSnapshot|TestLocalRedirectAndCustomCA)$' -v` | All selected checks pass. Fixed-operation cross-tenant denial, cancellation/revocation, endpoint pinning, DNS rebinding and redirect/CA behavior pass with local fixtures. |

No new regression was added: existing tests already exercise these local code paths.
The PostgreSQL corpus does not test browser artifact routes from a second authenticated
tenant; it tests artifact visibility through runner service/stream authorization.

## Limits and next evidence

- Process cgroup is `0::/`; `/sys/fs/cgroup` is root-owned and mode `0555`. Available
  controllers are `cpuset cpu io memory hugetlb pids rdma`, but `cpu.max`,
  `memory.max` and `pids.max` do not exist at this process's cgroup. Production runtime
  therefore remains closed by `NewRuntime`'s cgroup prerequisite.
- The passing gVisor run uses development/rootless mode with `--ignore-cgroups` behavior;
  it proves boundary and cancellation behavior, not enforced per-run resource limits or
  hosted tenant isolation. Do not enable hosted untrusted execution from this result.
- `TestPrivateConnectionProbeUsesEnrolledRunnerAndVault` requires explicit disposable
  Gitea fixture variables/token under `.local/gitea`; no Gitea container was running for
  this review. Real private Gitea probe remains unrun here.
- No hostile workload or 100 concurrently executing runs were launched. T28b records
  the read-only cgroup and nested-gVisor blockers plus required host setup. Repeat the
  hostile corpus and recovery/fairness tests on a runner with delegated writable cgroup
  v2, then observe 100 active per-run cgroups before submitting untrusted work.
- Live provider, official-agent subscription, hosted topology and customer credential
  certification remain external. No G5 claim follows from these local checks.
