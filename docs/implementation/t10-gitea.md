# T10 — Gitea adapter

Implemented against the running Gitea 1.27.3 OpenAPI document in `.local/gitea-api.json`. The adapter implements all frozen forge interfaces. Unsupported delivery and enforcement paths return explicit, actionable failures.

## Connection and controller wiring

`gitea.New(forge.Config)` requires a supplied fixed-origin HTTP client and operational token. Requests have a 15-second deadline, 4 MiB response/request limit and bounded pagination. Standard HTTP clients are cloned with redirects disabled. Custom clients must enforce the same origin and redirect policy. Provider error text never includes response bodies, credentials or request URLs.

`WithProtectionReader(reader)` returns a copy with a separate same-origin, same-organization inspector. Only branch-protection GET requests use this reader. The operational bot supplies identity, repository permissions, reviews, checks and every mutation. Gitea repository writers cannot read branch protection; the local inspector is a repository administrator with a token scoped to `read:repository,read:user`. No inspector means unknown protection, while inventory and PR operations remain usable. The controller must bind both connection versions/revocations and refresh both before dispatch.

`WithBranchAuthorizer(callback)` is mandatory for writes. Its full `UpdateBranchRequest` includes repository ID, branch, base SHA, expected old SHA, operation and edits. The controller must validate persisted task/operation ownership and live lease authority. The callback runs before every native write, including staging and final publication; callback absence denies writes. Branch prefixes, authors, commit messages and PR text are not ownership proof.

`WithCheckPublishers(map[context]actorID)` clones an exact-context-to-immutable-actor map. No wildcard contexts or login-derived actor identities qualify. A successful status from another publisher cannot replace the expected publisher's failure in adapter evidence. Gitea's native required contexts do not bind publishers, so those native rules remain unknown for automatic merge even when the observed publisher matches.

The merge controller must validate persisted `GateID`, operation, policy, scoped authority and evidence before calling the adapter. Merely supplying strings does not authorize a browser or worker to merge. Operation markers are reconciliation hints bound to repository ID, authenticated author, source and target repository IDs, and head/target branches; creation also requires the expected head SHA; persisted operation records remain authoritative.

## Qualified paths

| Path | Observed result on 1.27.3 |
| --- | --- |
| Branch append | Stage from immutable base; verify sole parent and complete tree; native `old_commit_id`/`new_commit_id` update with `force=false`. Concurrent branch advancement is rejected. |
| Branch creation | Stage and verify identically; native creation fails if the destination exists. |
| Fast-forward-only merge | Exact `head_commit_id`, `force_merge=false`, current official approval and strict branch protection. Target advancement at the final request boundary is rejected. Successful target and merge SHA equal the tested head exactly. |
| Ordinary merge | Native strict-target protection accepted a target change immediately before merge on the pinned server. This method, squash and rebase are excluded from automatic merge. |
| Required checks | Missing check prevents native merge. A different publisher's same-name success can override a trusted failure natively. Publisher-bound requirements block automatic merge. |
| Stale head/review | Both adapter and native API reject the stale head; native enforcement rejects an approval after the head changes. |
| Fork PR | Immutable head and target repository IDs remain distinct. Head statuses are read from the source repository at the exact head. |

Only 1.27.3 is qualified for guarded branch writes and fast-forward-only merge. Automatic merge additionally requires the repository to enable fast-forward-only, `block_on_outdated_branch`, a readable supported rule, and an operational actor without administrative/bypass authority. Rules expose only the certified merge-method intersection. Other versions remain unknown until their real contracts pass.

The adapter handles exact branch rules and the universal `*` rule in native priority order. An earlier complex pattern blocks effective-rule qualification. Approval team allowlists and bypass team membership also remain unknown. All native protection JSON contributes to the rules hash. The observed timestamp does not.

Gitea CODEOWNERS parsing uses Go regular expressions, negative patterns and Gitea escaping. `ReadCodeOwners` reads immutable target content in native precedence: root, `docs/`, `.gitea/`. Parsed owner references are visibility data; the adapter does not turn usernames into approval evidence or claim native enforcement. Presence of CODEOWNERS blocks the affected automatic merge until native owner enforcement is certified.

Staging refs are retained. Gitea's branch-delete API has no expected-old guard; unguarded cleanup could delete concurrently modified work. A controller may reconcile these refs with persisted ownership and an appropriately guarded deletion mechanism.

Native queue, protected Actions deployment, deployment artifact correlation and recovery remain uncertified. Those methods fail with explicit reasons; they do not downgrade to an unprotected dispatch. A qualified external CI or GitOps route can be supplied by its own adapter. No paid/external provider or Kubernetes mutation was performed. External G5 remains open.

## Validation

Run against the disposable local 1.27.3 server with existing admin, bot, reviewer and read-only inspector fixture tokens:

```sh
REFORGE_GITEA_TEST=1 REFORGE_TEST_ROOT=/home/mnorris/repos/reforge GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go test -race -count=1 ./internal/forge/gitea ./test/forge/gitea
```

Tests create uniquely named empty repositories and plain fixture files, then delete the repositories. They cover native target/head/review races, status spoofing, guarded branch CAS, fork identity, CODEOWNERS visibility, exact operation reconciliation, reviewer requests, webhook tampering, repository replacement, exhausted pagination, redirect handling and bounded/redacted HTTP errors. Real fixture tests are opt-in and require the exact pinned version.

Sources: [protected branches](https://docs.gitea.com/usage/access-control/protected-branches/), [CODEOWNERS](https://docs.gitea.com/usage/repository/code-owners/), [webhooks](https://docs.gitea.com/usage/repository/webhooks/), and the running server's OpenAPI document. Local observed contracts take precedence over assumptions about similar forge APIs.
