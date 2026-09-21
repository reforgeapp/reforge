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
evidence. Merge is never forced and branch protection is never bypassed.
