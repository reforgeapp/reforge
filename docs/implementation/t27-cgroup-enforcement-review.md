# T27 delegated cgroup enforcement review

Status: bounded local enforcement and SIGKILL orphan recovery passed on 2026-09-24. Kernel memory peak measured 12 KiB over configured `memory.max`; the [Linux cgroup v2 memory controller documentation](https://cdn.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html) permits temporary exceedance. This is recorded as a limit, not rounded away. No hosted or load qualification.

## Identity

- Successful run `1ad67d6f-db90-4c12-b79a-a48d90b1e369`, 2026-09-23 22:56:30–22:56:48 UTC (2026-09-24 Sydney). Harness return 0, cleanup return 0. A first run also cleaned successfully but harness returned 1 because its initial validator assumed `memory.peak <= memory.max`; actual cgroup readback and OOM events were correct, with the same 12 KiB peak overshoot. Validator now requires exact limit readback, OOM and group-kill events, empty cgroup, no remaining workspace processes, and records the measured overshoot.
- Repository `HEAD` during successful run `703306e26fe5383799b20899b199a64748d261e3`; current `HEAD` is `92e1d031dd18e4202128a91fa3805b2c49feacb7`. Sandbox, domain, probe and module source trees are byte-identical across those revisions and the local build inputs. Their Git object IDs are in [source-objects.txt](../../.local/opus-resume/cgroup-enforcement/source-objects.txt).
- Host: WSL2 Linux 6.18.33.2, 12 CPUs, 15 GiB RAM; Go 1.27.1. `runsc` `release-20260914.0`, SHA-256 `c0f4ec0ac1198975d5cf919a78f2302426de096f69eebd33e50125c3ca42d699`; image `sha256:8ea2b32a6d06546a04cb88ad3548b74c8cb3dcf12d98d811bf4fe19ddd2246e0`.
- Rebuilt offline from the listed source snapshot: sandbox tool SHA-256 `1f8aa13e75df771845fe2bcccb4c0b7438d72db03cbe29afa7cde219fdb12175`; sandbox probe `e75a8fc633959492391db51eda44305e3f497af45a835dc7fa6f4289d5ccd892`; guest load probe `25b8c9e6f8a6191d57df5865557402500c174c48802d7023ad0716d2b3398c93`; harness source `38aa84c20b4c851f225153e3768e53fd5d875f620e867efeec86427dab40ea66`; harness binary `a33e0927a96d389d75c5c7e79a5b09e65746f68c856fe9312cfffed7350e8116`. Full logs and JSON evidence are under `.local/opus-resume/cgroup-enforcement/runs/1ad67d6f-db90-4c12-b79a-a48d90b1e369/`.

## Topology and settings

```text
/sys/fs/cgroup/reforge-enforce-<uuid>   root:root; memory.max=1073741824; memory.swap.max=0
                                        cpu.max="100000 100000"; pids.max=192
└─ worker                              uid:gid 1000:1000; delegated manager leaf and runtime sibling
   ├─ manager                           harness process
   └─ runtime                           gVisor cgroup root
      └─ rf-<id>                        one guest at a time
```

Harness ran with `Development=false`, `Rootless=true`, untrusted workspace, network profile `none`, `MemoryBytes=268435456`, `CPUs=1`, `MaxProcesses=64`, `DiskBytes=16777216`, and 10-second command limits. Each guest cgroup read back `memory.max=268435456`, `memory.swap.max=0`, `memory.oom.group=1`, `cpu.max="100000 100000"`, `pids.max=64`. No preexisting aggregate, worker or host limit was changed by the harness. Writes of the same values to aggregate/worker limits and aggregate `cgroup.procs` failed with `permission denied`; aggregate ownership stayed `0:0`. Root `cgroup.subtree_control` read `cpuset cpu io memory hugetlb pids rdma` before and after.

## Results

| Scenario | Evidence |
| --- | --- |
| Basic isolation and tmpfs disk limit | Inspect passed (`host paths, environment and network isolated`); disk probe passed. Disk evidence concerns guest tmpfs, not host block I/O. |
| Memory exhaustion | Guest exited `-1`; post-use returned `ErrUnavailable`. Per-run `memory.events`: `max=24`, `oom=1`, `oom_kill=15`, `oom_group_kill=1`; cgroup emptied and was removed. `memory.max` read back exactly 268435456. `memory.peak=268447744`, 12288 bytes above limit. Aggregate memory peak 504451072; swap peak 0. |
| CPU pressure | Five-second, three-worker guest used 0.933 CPU per wall second under one-CPU `cpu.max`; `nr_throttled` increased by 2. Per-run cgroup removed. |
| PID pressure | Guest-generated fork pressure caused host tasks/threads managed by gVisor to reach the per-run `pids.peak=64/64`; `pids.events max` incremented once and runsc `WaitPID` returned EOF. Per-run cgroup removed. This measures this gVisor workload’s host-side task use; it does not imply one host task per virtual guest process. No separate host-side process-factory probe ran because kernel cgroup PID events already confirmed pressure. |
| Timeout cancellation | A 1.5-second hang returned timed out after 3.52 seconds; guest printed `descendant-started`; cgroup reached `populated 0`, no matching workspace processes remained, and `Destroy` removed it. |
| Caller cancellation | Parent cancellation returned in 35 ms; cgroup emptied, no matching workspace processes remained, and `Destroy` removed it. |
| Supervisor crash recovery | Fresh runtime was fenced while supervisor held the state lock. After SIGKILL, 22 host processes remained in the orphan cgroup. A new runtime removed the orphan cgroup and bundle; stale workspace was rejected with `ErrBoundary`. Recovery took 157 ms. A fresh inspect passed and cleanup/close succeeded. |

Harness plus cases took 18.4 seconds, below the five-minute outer deadline. Final aggregate peaks: memory 504451072/1073741824, swap 0, pids 73/192; aggregate CPU `nr_throttled=112`. No concurrent guest/load test ran.

## Cleanup and limits

Runner propagated harness and cleanup statuses (`harness_status=0 cleanup_status=0 final_status=0`). Cleanup used `cgroup.kill`, waited for aggregate `populated 0`, then removed owned groups deepest-first. The unique aggregate path and `/tmp/rfenf-1ad67d6f` scratch were absent afterward; no scratch process remained. Global subtree controllers matched before/after. No demo, database, unrelated cgroup, or external service was touched.

This is one WSL2 host run with a small probe corpus. PID limit was fully reached during pressure testing, so 64 does not establish build headroom. Memory enforcement killed the guest as configured, while observed `memory.peak` exceeded the nominal limit by 12 KiB. No concurrency/fairness, 100-run load, hosted topology, tenant isolation, or G5 claim follows from this evidence. Confidence in the recorded local results: 92%.
