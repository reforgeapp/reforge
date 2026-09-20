# Acceptance and qualification matrix

This is a release-evidence template, not a claim of implemented or tested capability. Record results in `progress.md` during implementation. Exact provider/model versions and runtime image digests belong in the resulting support matrix.

## Required scenarios

| ID | Scenario and expected result | Owner / gate |
| --- | --- | --- |
| V01 | Empty database install, migration, Go/Gin server and React production build succeed; fixtures cannot silently enable in production | T01, G0 |
| V02 | Cross-tenant and cross-team IDs, artifacts, search, SSE and pooled DB sessions cannot expose data; revoked access takes effect | T02/T05/T07/T27, G0/G5 |
| V03 | GitHub organisation, GitLab group/subgroups and Gitea organisation import asynchronously, retain stable identities and remove revoked access | T08–T11/T17, G1 |
| V04 | Private forge/model endpoint works only through its approved route; DNS changes, redirects and metadata destinations cannot expand scope | T03/T07/T11/T15/T27, G1/G5 |
| V05 | OpenAI, Claude, Google and a compatible self-hosted endpoint complete a tool loop; partial/malformed calls never execute; quota/cancel paths reconcile | T12–T15, G2 |
| V06 | Each enabled official-agent runtime proves pre-effect approval, isolated commands and credential custody; subscription eligibility is evidenced per account/topology | T16, G2/G5 |
| V07 | Existing Renovate/Dependabot grouped update is adopted; repair respects branch ownership and no duplicate update is generated | T18/T19, G2 |
| V08 | Broken baseline reproduced, bounded candidate fixed, original trusted validation passes; deleting a test or rewriting its script cannot manufacture success | T19, G2 |
| V09 | Process failure before/after PR creation and webhook replay converge to one observed change; uncertain provider responses are reconciled | T05/T19/T27, G2/G5 |
| V10 | Head/base/rule changes, spoofed check publisher, stale approval and incomplete pagination all prevent stale automatic merge | T08–T10/T21, G3 |
| V11 | GitHub queue, GitLab train and Gitea certified strict-update path obey their actual native semantics under concurrent outside merges | T08–T10/T21, G3 |
| V12 | Kill switch prevents new application actions; already-admitted native actions are observed/cancelled where supported without claiming guaranteed reversal | T06/T21/T27, G3 |
| V13 | Existing GitHub/GitLab deployment workflows preserve native approval gates and bind run, source revision, immutable artifact and environment | T22/T24, G4 |
| V14 | Gitea source and protected GitOps delivery repo promote an immutable digest through existing reconciler; no direct Kubernetes mutations | T23/T24, G4 |
| V15 | Pipeline completion without health evidence stays unverified; failed rollout stops expansion and recovery follows its own authorised operation | T22/T23/T25, G4 |
| V16 | Contending jobs cannot overspend reserved budget; unknown usage remains accounted for; subscription exhaustion cannot silently switch billing | T05/T16/T27, G5 |
| V17 | Malicious repo hooks, instructions, build scripts and symlinks cannot reach another tenant, model keys, forge credentials or host control plane | T07/T27, G5 |
| V18 | Canary failure/unverified health stops campaign; pinned repo set does not grow when a saved filter changes; other tenants receive scheduling time | T25, G5 |
| V19 | Self-hosted clean install, upgrade, encrypted restore and key recovery succeed; hosted runner drain/restart preserves state | T26/T27, G5 |
| V20 | J01–J10 with real backend: keyboard operation, dialog focus, readable gates, empty/error states, sanitised logs and cache reset on org switch | T04/T17/T20/T24/T28, G5 |
| V21 | 1,000 repos/org, 10,000 total, 100 concurrent jobs and 50 GUI sessions meet product latency targets without retry storms | T25/T28, G5 |
| V22 | Bounded maintenance evaluation corpus meets product criteria; zero policy bypasses; results broken down by recipe and model profile | T19/T28, G5 |

## External test environments

- Dedicated GitHub test organisation and separately certified GHES version where claimed; approval/queue/environment capabilities appropriate to the plan under test.
- Dedicated GitLab group and self-managed test instance; record edition/tier. Claims about approvals, trains and protected environments require those features to be enabled in the test configuration.
- Disposable pinned Gitea instance with protected branches and Actions where used; strict-update merge and portable GitOps promotion are independently exercised.
- Customer-owned BYO API accounts with explicit test budgets; at least one actual compatible self-hosted model per advertised protocol profile. Record unknown usage rather than assuming zero.
- Official-agent test accounts only when terms and topology eligibility are established. A working login is not entitlement evidence.
- Hosted runner infrastructure plus hostile fixtures and a private customer-runner network; no production customer source or secrets needed.

Local fixtures unblock development. They do not substitute for V10–V14 certification, subscription eligibility or the hosted sandbox proof.

## Evidence record

```text
Scenario / ticket / requirement:
Commit and build identity:
Provider/runtime versions, account capability and deployment topology:
Inputs and expected result:
Observed result and evidence artifact paths:
Pass / fail / not run:
Known limitation and affected feature gate:
Reviewer and date:
```

Do not store tokens, raw credentials or private customer source in evidence reports. Capture only the context needed to reproduce the test. A failed capability is disabled with a visible reason until retested; a missing v1 requirement keeps G5 open.
