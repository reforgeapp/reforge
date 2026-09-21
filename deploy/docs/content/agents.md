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
| Custom command profiles | Not enabled in this build | Requires an administrator-approved, digest-pinned profile; this build ships none |

## Why a route is disabled

A runtime is enabled only when a dated qualification record matches the exact runtime,
version, account, credential version, model, container digest and deployment topology.
Qualification covers entitlement, official authentication, headless operation, container
boundary, pre-effect approval, cancellation, output and event handling, usage accounting
and revocation.

A successful login is not entitlement. A plan name establishes nothing. The runtime is
never handed repository code without an isolation boundary, and it never receives OAuth
tokens from Reforge.

## Approval and cancellation

An approval request must be intercepted before the effect, not inferred from a
post-execution event. Cancellation is only confirmed by the runtime's native interrupted
state; EOF, timeout or a missing terminal leaves the outcome unknown and requires
reconciliation.

## Custom command profiles

The intended contract for an administrator-approved custom profile binds a fixed
executable and argv, a container image digest, a protocol version and the declared input,
event, output, cancel, exit and usage semantics, plus an approval record. A tenant cannot
submit an arbitrary shell or executable path. Until a profile is registered, approved and
its image digest pinned, the runtime stays unavailable with an actionable reason.
