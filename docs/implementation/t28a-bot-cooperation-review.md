# T28a bot cooperation acceptance

## Local result

`TestDiscoveryRenovateDependabotCooperation` scans two persisted native-change fixtures through discovery using real PostgreSQL. It reads Renovate and Dependabot configuration from immutable source snapshots, adopts each change only after its immutable actor ID is trusted, preserves provider change IDs and actor IDs, and blocks an untrusted Dependabot actor. After both actors are trusted, both findings remain bot-owned and carry the merge-authority conflict blocker. Their overlapping dependency groups fail repair admission for either finding; no repair binding or workflow task is created.

The scenario then moves Renovate's native head while retaining the same dependency group, rescans, checks the finding reflects the new head and version, and verifies a caller holding the previous version receives `discovery.ErrStale`.

Run against disposable PostgreSQL configured by `.local/development.env`:

```sh
set -a; source .local/development.env; set +a
go test -race -v ./test/integration -run '^TestDiscoveryRenovateDependabotCooperation$' -count=1
```

Result on 2026-09-23: **PASS**, 1.34s test time. The test container is the guarded local `reforge_test` database; no live provider or model API was called.

## Qualification boundary

This is local contract evidence, not certification of real Renovate or Dependabot accounts. Native changes, actor IDs, source manifests and provider reads come from deterministic test fixtures; bot processes, native merge queues and forge permissions are not exercised. Discovery verifies head at scan time and repair admission fences stale finding versions; there is no atomic provider-head check at a later publication boundary in this scenario. Publication is never invoked. V07/J02 therefore remain partial until supported GitHub, GitLab and Gitea native bot cooperation and protected publication paths are qualified with authorized disposable repositories.
