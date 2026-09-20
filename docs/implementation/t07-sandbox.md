# T07 — gVisor runtime boundary

The Linux runtime runs each attempt in a separate gVisor sandbox. Repository commands have no forge/model/broker credentials, host environment, writable host directory, runtime socket or host network. Source transfer is a trusted supervisor operation; the sandbox receives bounded files through a static helper over stdin.

## Runtime integration

`NewRuntime(RuntimeConfig)` validates pinned runsc/helper SHA-256 values, approved image digests and an exclusive `0700` state directory. Keep the returned runtime for the supervisor lifetime and call `Close()` at shutdown. Concurrent owners of the same state root are rejected. Shutdown attempts every workspace cleanup, aggregates failures and retains the state lock until cleanup succeeds. A workspace registered before shutdown cannot start afterward. Startup reconciles native container records only within that state root, kills orphaned sandboxes, removes their dedicated cgroups and deletes their recorded bundle directories.

Rootless mode uses a managed foreground `runsc --rootless run` process and polls native state for readiness. State, exec, kill and delete operations omit `--rootless`. Each invocation has a clean host environment and bounded output. Commands are argument arrays; the supervisor never invokes a host shell for repository commands. Every command must request `NetworkProfile: "none"`.

The approved toolchain image is read-only. `/workspace`, `/tmp` and `/dev` are isolated tmpfs mounts. HOME points into isolated `/tmp`; the image-owned read-only `/home` remains visible for toolchains whose ELF interpreter resides there. No host home is mounted. Workspace and temporary storage each use the configured disk ceiling; aggregate memory remains cgroup bounded. The trusted static helper is a separate read-only bind mount. Image directories `proc`, `dev`, `tmp`, `home`, `workspace`, `opt`, `opt/reforge` and regular placeholder `opt/reforge/tool` must exist before `ImageDigest` is computed; this prevents runsc from creating mount targets in the pinned image. Runsc, helper and image hashes are rechecked before workspace creation and each command/file operation. Images and executables are trusted supervisor-managed assets; host operators must keep them immutable during use.

`RuntimeConfig.Fetch` must retrieve an authorized immutable commit using the control plane's scoped source transport. It returns `Snapshot{CommitSHA, Complete, ManifestSHA256, Files}`. Use `SnapshotDigest(Files)` for the canonical manifest digest. The fetcher must verify the provider tree/archive against the requested commit and certify completeness before setting `Complete`. The manifest covers paths, executable bits and bytes; it does not itself prove Git commit ancestry or provenance. Browser-provided snapshots are not trusted source evidence. Limits are 20,000 files, 4 MiB per file and 64 MiB total; symlinks, devices and repository `.git` metadata are excluded.

Workspace lifetime includes fetch/startup and is bounded by the caller's context and requested timeout. File-operation locks honor caller deadlines; guest operations inherit workspace cancellation and have an additional 30-second ceiling. Artifact reads and patch writes use confined root handles, no-follow/nonblocking opens and opened-file regularity checks; writes validate before truncation. Artifact reads have a 4 MiB limit. Cancellation invalidates the workspace and terminates the entire sandbox, including descendants that created new sessions. `Destroy` performs bounded cleanup even after the task context expires.

Production requires an actual delegated cgroup-v2 directory. Each sandbox receives verified `memory.max`, `memory.swap.max=0`, `memory.oom.group=1`, `pids.max` and `cpu.max`; the supervisor uses `clone3` cgroup attachment before starting runsc, placing its descendants under the same limits. Missing delegation, unsupported attachment or failed limit writes prevent workspace startup. Cleanup uses `cgroup.kill` and native all-process termination.

The explicit combination `Development=true`, no cgroup root, and `Trust="development-fixture"` permits a local fixture without host cgroup enforcement. The local environment has no writable delegation, so production CPU/memory/PID enforcement and the hosted hostile-repository gate remain unqualified. Do not advertise hosted readiness from this fixture.

## Validation and remaining integration

The real local fixture passed with runsc `release-20260914.0` and `go test -race`. It checks missing production delegation, incomplete/corrupt snapshots, changed images, read-only mounts, separate workspaces, host environment/file/network denial, traversal/symlink/FIFO rejection, aggregate tmpfs exhaustion, bounded output, caller deadlines, actual sentry termination after a detached descendant, parent-context shutdown, restart orphan cleanup, recovery after a cleanup error and the registered-before-start shutdown race.

Build the two static fixture executables and run the actual namespace test:

```sh
CGO_ENABLED=0 GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go build -o bin/reforge-sandbox-tool ./cmd/sandbox-tool
CGO_ENABLED=0 GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go build -o /tmp/reforge-sandbox-probe ./test/sandboxprobe
REFORGE_TEST_RUNSC=/tmp/reforge-gvisor/bin/runsc REFORGE_TEST_SANDBOX_TOOL=/home/mnorris/repos/reforge/bin/reforge-sandbox-tool REFORGE_TEST_SANDBOX_PROBE=/tmp/reforge-sandbox-probe GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go test -race ./internal/sandbox -run TestRealGVisorBoundaryAndCancellation -count=1 -v
```

A successful command can leave background processes inside its attempt. They have the same workspace access as other repository code and can alter files while later commands run. T19 must use fresh workspaces and frozen baseline/candidate inputs for independent validation, and must bind recorded artifacts to the validated candidate. Guest tool output alone is not trusted validation evidence. Host secrets remain outside this boundary.

Runner enrollment, lease fencing, artifact authorization and private source/model routes are supervisor/control-plane responsibilities. Private Git transport qualification remains with T11/T19; no network access is granted to repository commands. Production delegation and hostile multi-tenant qualification require a dedicated runner fixture in T27.

Sources: [gVisor rootless lifecycle](https://gvisor.dev/docs/user_guide/rootless/), [gVisor security architecture](https://gvisor.dev/docs/architecture_guide/intro/), [Linux cgroup v2](https://docs.kernel.org/admin-guide/cgroup-v2.html), and the pinned runtime's command help.
