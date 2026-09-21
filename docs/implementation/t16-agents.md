# T16 official agent bridges

Codex app-server 0.150.1 has a concrete Go JSONL bridge implementing `AgentExecutor`. Production subscription routes remain disabled until an exact deployment binding has current qualification evidence. No existing account cache was read or copied, no real account was inspected, and no live login, logout or paid turn was performed.

## Codex protocol and broker

`NewCodex(CodexConfig)` accepts a fixed organization, connection/credential version, runner, immutable account reference, model, runtime digest and deployment binding. `Open` supplies the trusted isolated official runtime transport with the same binding. `Qualify` must read a controller-owned evidence record with matching binding, UUID, verification time and expiry. Browser-supplied attestations do not qualify a route.

The bridge initializes once, checks the pinned version, enables the experimental dynamic-tool protocol and reads account state without proactive refresh. It accepts only managed ChatGPT authentication. API keys, externally supplied ChatGPT tokens, cloud-provider auth and automatic billing fallback are unsupported. Qualification reports separate account custody, native tool containment, terms, topology, entitlement, quota and paid-overage decisions. A ChatGPT plan name alone establishes none of them.

One bridge admits one native turn for its pinned job/attempt and workspace. `MaxTurns` must be one. `thread/start` fixes the model/provider, read-only native sandbox, no native sandbox network, user approval reviewer and untrusted approval policy. Returned settings must match. Native command, file-change and permission requests are declined before execution; policy or human approval cannot enable those host operations. Post-effect command/file output, unknown effect requests, tool invocations disguised as notifications and ambiguous protocol state close the runtime with an uncertain outcome.

Three dynamic tools are registered: `reforge_read`, `reforge_patch` and `reforge_exec`. Complete arguments pass bounded schemas and path/argv checks before authorization. All repository reads, patches and commands use the supplied `SandboxRuntime`, never host filesystem or process APIs. Execution has no network, at most 60 seconds and 256 KiB output. Patch data and file results are bounded. The default tool-call limit is 32, with a hard maximum of 128. Native call IDs are one-use within the attempt; controller operation IDs are generated separately.

`Authorize(ctx, Effect, perform)` is a trusted synchronous controller callback. Before calling `perform`, it must revalidate current job/attempt fencing, policy, workspace/SHA/image, connection/credential/account versions and the configured official billing route. It must commit the durable dispatch/side-effect claim and required quota/budget reservation before any paid turn or sandbox effect. A claim made only in a transaction that can roll back after the effect is insufficient. The callback must invoke `perform` exactly once under the required current authorization; callback failure after invocation is uncertain. No automatic retry or API fallback occurs.

Broker work waits until the initial authorization callback returns successfully. Native completion cannot overtake outstanding broker work. Terminal usage is explicitly unknown, including on quota failures. `Cancel` sends `turn/interrupt`; only the native interrupted terminal establishes cancellation. EOF, timeout, callback failure or missing terminal requires reconciliation. `ResumeIfSupported` reattaches only the same known completed/interrupted native thread after fresh authorization, rechecks returned settings and never starts another paid turn. Process-loss/uncertain sessions cannot resume automatically.

Managed login/logout are separate methods requiring a UUID-bearing explicit user action and a trusted `AuthorizeUserAction` callback. They invoke only official managed methods; the bridge never receives OAuth tokens or opens login URLs itself. Login is denied while work is active. Authentication changes invalidate this bridge for new turns; rebuild its binding only after fresh account/credential qualification. Login URLs are returned only for the explicit sign-in flow and are redacted in formatted/structured logs.

## Runtime custody and limits

`StartPreparedRuntime` supplies bounded stdio and process-group cleanup for a supervisor-prepared command. It is not an isolation boundary or a production launcher configuration. It accepts an explicit small environment and never inherits provider keys, user authentication variables or shell initialization. Its command must come from the trusted namespace/container factory, never a browser, repository or model. No shell command or generic process endpoint is exposed through the bridge.

The production factory must isolate the official runtime's account store, credential-bearing process memory, configuration, plugins/hooks, native filesystem reads, subprocesses and network from repository code and other customer workspaces. Runtime version and binary digest must remain pinned. Native read-only sandbox settings and approval callbacks alone do not prevent implicit native reads of credentials. Process-group cleanup alone does not contain detached descendants; qualify the external namespace/cgroup lifecycle. The credential-holding runtime and untrusted repository sandbox must remain separate. No `HOME` or `CODEX_HOME` override is used here.

JSONL frames are capped at 1 MiB, protocol traffic at 32 MiB and stderr at 256 KiB. Request/notification queues are bounded. RPC response waits are capped at ten seconds; writes at five seconds. Attempt wall time defaults to ten minutes and cannot exceed one hour. Unknown provider usage never becomes zero spend.

A native turn may contain multiple internal model requests. `MaxTurns=1` does not provide a token or request ceiling. Quota qualification must establish effective provider/runtime limits and disable paid credits/overage; otherwise the route stays disabled. The pinned `account/read` response lacks an immutable account ID. That identity must be established through independently verified official-runtime credential custody and versioned controller evidence, not inferred from email or plan text.

## Provider support matrix

| Route | Status | Required qualification |
| --- | --- | --- |
| Codex managed app-server | Bridge implemented; no production deployment qualified | Exact customer account/runtime/model binding, isolated custody and native tools, entitlement, terms, topology, quota and no paid overage |
| Claude Code official binary | Disabled; no bridge activated | Unmodified binary and native user authentication, direct end-user billing, applicable commercial conditions, and proved pre-effect isolation |
| Claude Agent SDK subscription login | Disabled | Explicit provider permission for the intended third-party product; API-backed adapters remain a separate billing route |
| Google Antigravity CLI `agy` | Disabled; no bridge activated | `agy` is a distinct runtime from Gemini CLI. Pinned headless protocol, account custody, pre-effect approval and container isolation for the exact topology |
| Gemini CLI official binary | Disabled; no bridge activated | Customer plan/deployment permission, official login custody and proved pre-effect isolation |
| Direct use of CLI OAuth credentials | Unsupported | Never extract, store or reuse CLI session tokens in model adapters |

The matrix distinguishes documented binary-hosting conditions from an authorization to intermediate subscription usage. It does not claim blanket hosted subscription eligibility.

Official sources checked: [Codex app-server](https://learn.chatgpt.com/docs/app-server), [Codex authentication](https://learn.chatgpt.com/docs/auth), [Claude Code legal and compliance](https://code.claude.com/docs/en/legal-and-compliance), [Claude Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview), [Gemini CLI terms](https://geminicli.com/docs/resources/tos-privacy/) and [Gemini CLI authentication](https://geminicli.com/docs/get-started/authentication/). Protocol field checks use the installed 0.150.1 experimental schema, not assumptions from a newer documentation example.

## Validation

Race-enabled JSONL subprocess fixtures cover pre-effect durable claims, broker-only execution, declined native approvals, post-effect/false-callback rejection, traversal and unknown tools, premature terminal messages, native cancellation, resume without replay, quota/billing distinction, account-binding rejection and explicit managed login/logout. These fixtures do not certify provider entitlement or isolation.

The actual installed binary reported `codex-cli 0.150.1`; its generated experimental schema contains the required dynamic-tool and approval fields. Initialization against the installed account environment was deliberately excluded because no isolated account-store namespace has been qualified. There is no claim of a live account, paid-turn, hostile-repository or subscription-entitlement test.

```sh
REFORGE_TEST_CODEX=/home/mnorris/.local/bin/codex GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go test -race -count=1 ./internal/agent
```

Root integration owns the isolated runtime factory, durable authorization/accounting, account-qualification records and explicit user-action HTTP routes. External account/custody/entitlement certification remains open.

Coordinator reviewed all bridge, protocol, process and fixture source. Added close-during-startup protection so a late runtime cannot initialize or reopen a closed bridge. The final root race run and pinned binary/schema check are recorded in progress.md.

## 2026-09-21 planning addendum

Agent choices are separate qualification targets: Claude Code, Codex and Google Antigravity CLI `agy`; `agy` is not Gemini CLI. The [Antigravity headless CLI reference](https://antigravity.google/docs/cli/headless/) documents a technical JSON/NDJSON headless surface only. It does not establish subscription entitlement, credential custody, container isolation or approval interception.

Enable a runtime/version/topology only after dated evidence covers entitlement, official authentication, headless operation, container boundary, pre-effect approval, cancellation, output/event/usage handling and revocation. Unknown or unverified status stays disabled with actionable reason. Never convert subscription credentials into API keys or silently switch to a paid API route. T31 custom profiles follow the same gate and bind fixed executable/argv, image digest, protocol and policy/run identity.
