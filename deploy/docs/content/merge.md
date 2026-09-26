# Changes and merge

The **Changes** route shows native pull requests and merge requests with application
ownership, the original dependency update, target branch, author, latest head, native
check and review status, Reforge validation and the effective policy.

## Eligibility panel

Each gate shows its own source:

```text
Candidate validation       Passed at a81d2f
Required CI checks         6/6 passed — GitLab
Code owner review          Awaiting platform team — GitLab
Target freshness           Current against e922b0
Reforge change policy      Low risk; paths within allowance
Merge route                Native merge train
Decision                   Waiting for code owner review
```

Unknown rules render as “Cannot verify protection; automatic merge blocked”. No merge
control is enabled while a gate is pending or unknown.

## Reforge-enforced gate

With autopilot on and no merge configuration, Reforge enforces the gate itself on GitHub
branches that have no protection, or whose protection the connection's identity can bypass.
The gate is recorded as `reforge-enforced`:

- every check passing on the target branch must pass on the pull request;
- the pull request must not be behind the current target branch;
- the merge pins the tested head;
- a native "changes requested" review, conflicts or a draft still block.

Native review requirements are satisfied by this gate. Branches the identity cannot bypass
use the native rules.

## Dependency bots

Autopilot merges open pull requests from trusted bots through the same gate. When a
Dependabot pull request falls behind, Reforge comments `@dependabot rebase` and merges once
the rebased pull request passes. When the default branch's own CI fails, bot pull requests
failing the same checks wait while Reforge fixes the default branch first.

## Exact revisions

Merge evaluation binds the head, target and tested revisions. If any of them move, the
evaluation is stale and must be refreshed. A spoofed check publisher with the same name
cannot override a trusted failure.

## Native queues and trains

GitHub queue and GitLab train admission use the provider's native mechanism. Enrolment in
a queue or train is shown as pending external work, not as merged. A native approval is a
provider review, not an application approval, and the interface labels the difference.

## Bot cooperation

When a companion change must merge first, Reforge records the order and revalidates the
original bot update after the companion merges, using fresh head, target and native gate
evidence. Merge is never forced.
