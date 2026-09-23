# T27 delegated cgroup probe review

Status: one production-config (`Development=false`, `Rootless=true`) gVisor guest plus
timeout-cancellation, caller-cancellation and graceful close/reopen passed on 2026-09-24 inside a
disposable delegated cgroup v2 subtree. Not a load, hostile-corpus or hosted qualification.

## Identity

- Repository `HEAD` `bb6aebd95d4b7a114f3a894b46162e259eb486ea` (clean for `internal/sandbox`).
  Sandbox source SHA-256: `runtime_linux.go` `cfb9fa16…ecca1`, `process_linux.go` `bf36ffd9…7453`,
  `guest/guest.go` `cc2c21b9…efee`.
- Host: WSL2 Linux 6.18.33.2, 12 CPUs, 15 GiB RAM, cgroup2 mounted `nsdelegate`; Go `go1.27.1`.
- `runsc` `release-20260914.0` `c0f4ec0a…d699`; helper `bin/reforge-sandbox-tool` `799bf8c8…35b2`
  (built from `2bfd7f8`, `vcs.modified=true`); probe `/tmp/reforge-sandbox-probe` `1a4e2ce2…89cc`
  (built from `a03538e`, clean; `test/sandboxprobe` unchanged since). Same assets as
  [t27-current-host-review.md](t27-current-host-review.md). Image digest
  `sha256:5171825f2ed58c7eb2bb3506a7b8a849a10517bf7a9a26b6b86720ed92b55e34`.
- Harness `.local/opus-resume/cgroup-probe/harness/main.go` `85c4f7cf…73cb`, binary `aece7b86…9b33`,
  built offline outside the aggregate:
  `GOPROXY=off GOFLAGS=-mod=readonly CGO_ENABLED=0 go build -overlay .local/opus-resume/cgroup-probe/overlay.json -o .local/opus-resume/cgroup-probe/bin/reforge-cgroup-probe ./cmd/reforge-cgroup-probe`
  (overlay maps a virtual `cmd/` path; no product tree change).
- Run `d5452915-2825-425d-92b6-5b3684d7b103`; artifacts
  `.local/opus-resume/cgroup-probe/runs/d5452915-…/{run.log,harness.jsonl,result.json}`.

## Topology

```text
/sys/fs/cgroup/reforge-probe-<uuid>        root:root  memory.max=1073741824 memory.swap.max=0
                                                      cpu.max="100000 100000" pids.max=192
                                                      cgroup.max.descendants=16 max.depth=4
                                                      subtree_control="cpu memory pids"
└─ worker                                  dir, cgroup.procs, cgroup.subtree_control, cgroup.threads -> 1000:1000
   ├─ manager                              chown -R 1000; harness leaf
   └─ runtime  (CgroupRoot, harness mkdir) subtree_control="cpu memory pids", no processes
      └─ rf-<id>                           created by sandbox.Runtime per workspace
```

Commands (`.local/opus-resume/cgroup-probe/run.sh`):

```sh
sudo -n sh setup.sh "$U" 1000
sudo -n timeout -s KILL 300 sh -c 'P=$1; shift; echo $$ > "$P/worker/manager/cgroup.procs" && exec setpriv --reuid=1000 --regid=1000 --clear-groups env -i PATH=/usr/bin:/bin HOME=/nonexistent "$@"' sh "$P" bin/reforge-cgroup-probe -runsc … -tool … -probe … -scratch /tmp/rfcg-<8> -worker "$P/worker" -out runs/$U -memory 268435456 -pids 64
sudo -n sh cleanup.sh "$U"
```

Launcher wrote only its own new `$$` (later exec'd harness, pid 24368) into `manager`; no other
process migrated (aggregate `cgroup.procs` empty). Root `cgroup.subtree_control`
(`cpuset cpu io memory hugetlb pids rdma`) identical before/after. Docker and other cgroups untouched.

Runtime config: `MemoryBytes=268435456`, `DiskBytes=16777216`, `CPUs=1`, `MaxProcesses=64`,
`StateRoot=/tmp/rfcg-<8>/state`, `Trust="untrusted"`, workspace timeout 2m.

## Results

| Check | Observed |
| --- | --- |
| Harness identity | uid/gid 1000, no supplementary groups, `CapEff=0`, cgroup `…/worker/manager` |
| Aggregate root ownership | Harness writes (same value) to aggregate `memory.max`, `pids.max`, `cgroup.procs` and `worker/memory.max`, `worker/pids.max`: all `permission denied` |
| Delegation | Harness enabled `worker`/`runtime` `subtree_control`; unprivileged `CLONE_INTO_CGROUP` into `runtime/preflight` succeeded (child `/proc/self/cgroup` confirmed). Common ancestor `worker` owned by uid 1000; root `cgroup.procs` not chowned |
| `NewRuntime` production | Accepted; no `ErrUnavailable` |
| Per-run readback | `memory.max=268435456`, `memory.swap.max=0`, `memory.oom.group=1`, `pids.max=64`, `cpu.max=100000 100000` |
| Membership | 15 procs at ready: `runsc run`, re-exec, `runsc-gofer`, `runsc-sandbox` (`gvisor_sentry`, 17 threads), `runsc-fd-parking`, 10 systrap stubs; 21 during `hang` incl. `runsc exec`. All in `rf-<id>`, uid 1000. No process with state root or workspace ID outside |
| Basic guest | `inspect` exit 0 `host paths, environment and network isolated`; `read` = `baseline` |
| Timeout cancel (`hang`, 1.5s) | `TimedOut=true`, `descendant-started`; call returned 3.51s; cgroup `populated 0` at first check; post-use `ErrUnavailable`; `Destroy` removed cgroup |
| Caller cancel | Parent ctx cancel after 1.5s: call returned 7ms, cgroup empty, `Destroy` removed cgroup |
| Graceful reopen | `Close` then new `NewRuntime` on same state/cgroup root; new guest `inspect` exit 0; `Destroy`/`Close` clean; `runtime` empty |
| Per-run peaks | `memory.peak` ≤37.6 MiB, `pids.peak` 54/64, `memory.events`/`pids.events` all 0 |
| Aggregate peaks | `memory.peak` 280219648, `memory.swap.peak` 0, `pids.peak` 62/192, `nr_throttled` 12 (`throttled_usec` 547586), no max/OOM events |
| Cleanup | `cgroup.kill` on owned subtree (already `populated 0`), `rmdir` deepest-first: `manager`, `runtime`, `worker`, aggregate. No `reforge-probe-*` left; scratch `/tmp/rfcg-d5452915` removed; no scratch processes |

## Limits and blockers

- Pids headroom: idle guest uses 45–49 host tasks, sleep workload 54 of 64. Systrap stubs and
  `runsc exec` count against `pids.max`; real builds will likely exceed 64. Size
  `MaxProcesses` from a measured build corpus before hosted use.
- Timeout return lags by ~2s (3.51s for 1.5s), consistent with `cmd.WaitDelay`; bound
  observed, not a leak.
- Aggregate `memory.peak` includes harness and page cache from hashing the 109 MB `runsc`
  per operation; per-run guest peak is ~37 MiB.
- Crash/orphan recovery was not exercised; `Close` deliberately cleaned prior work before reopen.
- Enforcement not exercised: no memory/pids/cpu exhaustion workload, OOM-group kill, fairness
  or 100 concurrent runs. Hostile corpus not run.
- Delegation layout came from this probe's root setup script; product has no installer/unit
  (e.g. systemd `Delegate=`) producing it. Production launcher must create the equivalent
  manager leaf and sibling `CgroupRoot` before hosted enablement.
- Helper binary built from modified tree at `2bfd7f8`, not rebuilt from `HEAD`.
- WSL2 single host only; no hosted topology, tenant or G5 claim.

Confidence in recorded evidence: 90%.
