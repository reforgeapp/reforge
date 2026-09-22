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
| Administrator-approved custom command profiles | Implemented; disabled until an administrator approves a digest-pinned profile; see [Agent runtimes](agents.md) |
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

`REFORGE_LOAD=1 go test -run TestControlPlaneLoadTargets ./test/integration/` exercises a
10,000-repository asynchronous inventory import, scoped pagination and search, 50
concurrent authenticated sessions and 100 queued task claims against disposable
PostgreSQL. Representative local run on this host (Go 1.27, PostgreSQL 18.6, single
process, shared development machine): scan `2.9s`, import `10.5s`, first scoped page
`6ms`, filtered search `5ms`, 50-session p50/p95/max `19/21/22ms`, 100 claims `2.2s`.

These are control-plane measurements with a simulated provider. They measure claim
lifecycles, not 100 fully executing runs, and 50 authenticated HTTP sessions, not 50 real
browser sessions. They do not certify live providers or hosted execution. Bounded campaign
fairness across a 1,000-member tenant and 100 competing runner claims are covered by the
integration suite; a 100-executing-run and 50-browser-session gate remains open (T28).

## Custom command profiles

An administrator-approved profile binds a pinned image digest, fixed executable and argv,
protocol version and declared limits. A repair selects it through a `custom_command`
agent connection and quota budget route; the controller revalidates the task fence,
policy, approval/version/digest and concurrency, commits a durable reservation and run
record, and the runner executes the profile in its sandbox. The version 1 protocol is a
single bounded invocation: `max_turns` is declared and validated but not looped, and a
profile run records `handoff`, never a validated repair. Revocation fences later
dispatches.

## Verification identity

| Component | Version exercised locally |
| --- | --- |
| Go | 1.27 toolchain |
| Node / npm | 26 / 11 |
| PostgreSQL | 18.6 |
| React / Vite / TypeScript | 19.3 / 8.3 / 5.9 |
| Gitea (disposable) | 1.27.3 |
| Compatible model endpoint | Ollama with a small Qwen model |
| Browser | Chromium via Playwright |

The encrypted restore drill restores a disposable database and decrypts a sealed
envelope; it does not prove provider credentials still work. The hosted GitOps reference
under `deploy/gitops` renders manifests but does not certify a hosted cluster.

## Licences

Reforge is Apache-2.0. The runtime dependency set has no GPL/AGPL/LGPL module; the only
weak-copyleft licences are MPL-2.0 build/test dependencies (`axe-core`, `lightningcss`),
which are compatible with an Apache-2.0 distribution when their notices are retained. The
generated inventory at `docs/implementation/dependencies.md` and collected notices at
`docs/implementation/third-party-notices.txt` mark transitive modules without a detected
licence file as `review required`; confirm those before a public release. No dependency
adds telemetry or phones home.

## Known limitations

- Hosted untrusted execution requires a sandbox host with resource enforcement. On hosts
  without working cgroup delegation, the runner reports the sandbox as unavailable and
  hostile repositories must not be admitted. `make sandbox-test` runs the hostile corpus
  (path traversal, symlink escape, truncated/corrupt fetch, cancellation, boundary) under
  a rootless gVisor sandbox; it passes locally with development resource limits and does
  not substitute for hosted cgroup-enforced isolation.
- Migration, backup and credential restore procedures are operator steps; see
  [Backup and restore](backup-restore.md).
- The interface does not perform infrastructure installation, Kubernetes changes, or
  provider-native login and approval. Those use documented handoffs.
- A 10,000-repository, 100-concurrent-run load gate and the non-development Compose
  install (which needs an OIDC issuer and HTTPS origin) are not yet recorded.
- Live forge, paid model and official-agent account certification remain outstanding; the
  affected routes stay disabled or uncertified rather than silently falling back.
