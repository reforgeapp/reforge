# Agent runtimes

Agent choices are distinct: Claude Code, Codex and Google Antigravity CLI `agy`. `agy` is
not Gemini CLI. Direct reuse of CLI OAuth credentials is unsupported in every case.

## Status in this build

| Runtime | Status | Reason |
| --- | --- | --- |
| Codex managed app-server | Disabled for production | Bridge implemented; no deployment binding has current qualification evidence |
| Claude Code official binary | Disabled | No unmodified binary binding with proved pre-effect isolation |
| Google Antigravity CLI `agy` | Disabled | No qualified headless, custody or entitlement evidence |
| Gemini CLI | Disabled | Direct CLI credential reuse is unsupported |
| Custom command profiles | Implemented; disabled until approved | An administrator creates and approves a digest-pinned profile; no profile is enabled by default |

## Why a route is disabled

A runtime is enabled only when a dated qualification record matches the exact runtime,
version, account, credential version, model, container digest and deployment topology.
Qualification covers entitlement, official authentication, headless operation, container
boundary, pre-effect approval, cancellation, output and event handling, usage accounting
and revocation.

A successful login is not entitlement. A plan name establishes nothing. The runtime is
never handed repository code without an isolation boundary, and it never receives OAuth
tokens from Reforge.

## Recording qualification

Open an agent connection in **Connections** to see its runtime capability state. An owner
or administrator records a dated qualification against the exact connection binding
(runtime version, account/workspace, model, deployment) with an evidence reference and
the seven checks above. The capability list updates immediately; an expired or missing
record leaves the route disabled with the missing check named. Clearing the record or
changing the connection binding disables it again.

A feature check does not certify the runtime itself: the runtime entry stays
`unsupported` until a qualified deployment binding exists for it.

## Runtime delivery

Reforge ships the bridge, custody checks and managed login/logout; the official runtime
binary or image is supplied by the operator. Configure one of:

- `REFORGE_AGENT_RUNTIME_IMAGE` — a container reference pinned by digest
  (`repo/name@sha256:...`). The runtime runs with no network, a read-only root, a small
  tmpfs and a non-root user. `REFORGE_AGENT_RUNTIME_EXECUTABLE` selects the executable
  inside the image (default `/app/codex`).
- `REFORGE_AGENT_RUNTIME` — an absolute host binary path, optionally pinned with
  `REFORGE_AGENT_RUNTIME_SHA256`. Reforge refuses to start a binary whose digest does not
  match.

Neither launcher passes provider keys, user authentication variables or shell
initialisation to the runtime, and no credential is baked into an image. A container is
not hosted tenant isolation: hosted untrusted execution still requires the runner host
namespace/cgroup boundary described in [Security model](security.md). With no launcher
configured, every official runtime stays disabled and login/logout are refused.

## Approval and cancellation

An approval request must be intercepted before the effect, not inferred from a
post-execution event. Cancellation is only confirmed by the runtime's native interrupted
state; EOF, timeout or a missing terminal leaves the outcome unknown and requires
reconciliation.

## Custom command profiles

A custom command profile binds a fixed executable and argv, a pinned container image
digest, a protocol version and declared wall-clock, output, turn and concurrency budgets.
An owner or administrator creates a draft and approves it with an evidence reference; the
profile can be revoked at any time, which fences future runs.

The runner executes only the approved image digest with the fixed argv. A tenant cannot
submit an arbitrary shell, executable path or interpolated argument. Version 1 of the
protocol reads one JSON input on stdin and emits newline-delimited JSON events; a malformed
or unknown event, oversized output or timeout leaves the outcome `unknown` and requires
reconciliation. A nonzero exit is a failure, and **exit 0 is never a validated repair** —
the declared validation still has to pass. Usage is recorded only when the profile reports
it; otherwise it stays unknown and the reservation is held.

Profiles are managed in **Connections → Custom command profiles**. Until a profile is
approved, it stays disabled with an actionable reason.
