# Support matrix and limitations

This page records what is implemented and what is certified. Local verification against
disposable fixtures is not the same as certification against a live provider. A missing
external credential does not make the feature unavailable to a configured deployment; it
means this build has no recorded certification for that provider.

## Forges

| Provider | Import and discovery | Publication | Protection and merge | Certification |
| --- | --- | --- | --- | --- |
| GitHub (cloud and GHES) | Implemented | Implemented, guarded | Protection, rulesets, queue gate implemented | Live cloud/GHES certification outstanding |
| GitLab (cloud and self-managed) | Implemented | Implemented, guarded | Approvals, train gate implemented | Live certification outstanding; tier/edition evidence required |
| Gitea | Implemented | Implemented, guarded | Strict-update merge implemented | Certified against the disposable pinned instance only |

Gitea merge results depend on the deployed server version and protected-branch
configuration. A capability that cannot be verified stays disabled for the affected
action and shows the reason.

## Models and agents

| Route | Status |
| --- | --- |
| OpenAI, Anthropic, Google direct APIs | Adapters implemented; paid live certification outstanding |
| Compatible self-hosted (Chat Completions; Responses probed) | One real local endpoint qualified per profile |
| Codex managed app-server | Bridge implemented; production route disabled pending account, custody, topology and quota qualification |
| Claude Code official binary | Disabled; no qualified deployment binding |
| Google Antigravity CLI `agy` | Disabled; `agy` is not Gemini CLI and has no qualified binding |
| Administrator-approved custom command profiles | Disabled; see [Agent runtimes](agents.md) |
| Gemini CLI | Disabled; direct CLI credential reuse is unsupported |

Subscription routes are enabled only for a documented, permitted, qualified runtime,
account and topology. Unknown or unverified combinations are disabled with a reason and
never silently billed as API usage.

## Delivery

- Native GitHub and GitLab workflows are orchestrated with native approval visibility.
- Portable GitOps promotion is exercised against a disposable Gitea delivery repository.
  Live reconciler health certification is outstanding.
- "Pipeline completed" is reported separately from "application verified". A workflow
  without signed health evidence is never rendered as healthy.

## Scale

Asynchronous inventory import, bounded campaign fairness and 100 concurrent runner
claims are exercised in tests. A 10,000-repository, 100-concurrent-run load gate is not
yet recorded.

## Known limitations

- Hosted untrusted execution requires a sandbox host with resource enforcement. On hosts
  without working cgroup delegation, the runner reports the sandbox as unavailable and
  hostile repositories must not be admitted.
- Migration, backup and credential restore procedures are operator steps; see
  [Backup and restore](backup-restore.md).
- The interface does not perform infrastructure installation, Kubernetes changes, or
  provider-native login and approval. Those use documented handoffs.
