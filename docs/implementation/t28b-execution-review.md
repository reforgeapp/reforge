# T28b execution load review

Status: 100 executing runs not exercised on 2026-09-23. No safe host-level resource boundary is available to this process; no load was started.

## Local capacity evidence

- Host reports 12 logical CPUs, 16,352,161,792 bytes total RAM, and 13,523,325,696 bytes available RAM at inspection. Repository filesystem reports 23 GiB free.
- Docker reports cgroup v2 with `cgroupfs`; daemon security options include `cgroupns` and seccomp.
- Process cgroup is `0::/`. `/sys/fs/cgroup/cgroup.controllers` lists `cpu cpuset io memory hugetlb pids rdma`, while `cpu.max`, `memory.max`, and `pids.max` are absent. The mounted cgroup root is not writable by this process (mode `0555`).
- Production `internal/sandbox.NewRuntime` rejects missing production cgroup configuration. Per-workspace setup writes and verifies `memory.max`, `memory.swap.max`, `pids.max` and `cpu.max` before execution.
- Development gVisor uses `--ignore-cgroups`; its configured memory, CPU and process limits therefore do not prove enforced limits on this host. Running 100 such workers could exhaust host resources. Docker's own cgroup support does not give this process a delegated subtree for Reforge's per-run sandbox.

## Bounded Docker alternative

A no-network Docker container with `--memory=1g --cpus=2 --pids-limit=128` exposed those outer limits (`memory.max=1073741824`, `cpu.max=200000 100000`, `pids.max=128`). Its cgroup mount was read-only; creating a nested cgroup failed with `Read-only file system`. One rootless gVisor probe inside that container, with only read-only mounts for the existing probe assets and no privileged flags or Docker socket, failed before guest start: `fork/exec /proc/self/exe: operation not permitted`. Container stopped at command exit. Outer Docker limits bound the whole runner process but cannot supply independently enforced Reforge per-run cgroups; this route cannot certify the required execution slice.

## Existing evidence and gaps

- `test/integration/load_test.go::TestControlPlaneLoadTargets` enqueues 100 jobs and obtains 100 workflow leases. It does not run them. Do not count it toward the 100-run requirement.
- `test/integration/repair_live_test.go` exercises one runner worker and one real repair. It does not demonstrate execution overlap or load fairness.
- `docs/implementation/t27-qualification.md` records earlier rootless gVisor boundary and cancellation feasibility checks, plus 100 competing claims. Those checks do not show cgroup-enforced resource isolation or 100 simultaneous executions.
- No load harness or database workload was started. The single nested-runtime feasibility probe did not start a guest. Existing demo containers and data were left untouched.

## Evidence needed to close T28b/T27

Provide a disposable Linux runner host with delegated writable cgroup v2 subtree and enough measured capacity for 100 configured run limits plus control-plane overhead. Confirm the runner can create a child cgroup and read back all four limit files before submitting work. Use a disposable tenant/repository set and fixed no-network workload with explicit CPU, memory, PID, output, disk and wall-clock bounds. Capture per-run start/end and cgroup IDs so 100 active executions are independently observable at one timestamp; record task transitions, per-tenant admission order/fairness, cancel-to-process-exit time, runner/server restart recovery, cgroup peak usage, host load, and request/run latency. Repeat after cancellation and restart; retain raw machine-readable reports and exact build/host identities. Test failure paths for rejected or unreadable cgroup limits. Then run the hostile/cross-tenant/budget corpus on the same enforced boundary. Do not promote development gVisor results to hosted certification.
