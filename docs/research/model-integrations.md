# Model and agent integrations

Research date: 2026-09-20. Primary documentation was fetched on this date. This is an engineering integration decision, not a legal opinion. Provider eligibility, billing, runtime versions, and policies must be revalidated before release.

## Decision

Keep the control plane, scheduler, policy engine, worker supervisor, and direct model loop in Go. Ship native OpenAI, Anthropic, and Google API adapters plus an explicitly profiled OpenAI-compatible endpoint adapter. Add official agent bridges as optional worker runtimes. Prefer an eligible customer's existing subscription/workspace entitlement when its documented use fits the deployment; otherwise offer customer-owned API credentials without silently switching billing.

Separate three products in the connection model:

| Connection | What it provides | Credential owner | Execution owner |
| --- | --- | --- | --- |
| Direct model API | Generation and requested tool calls | Customer API/cloud account | Our Go loop and policy-controlled tools |
| Official agent bridge | Provider agent loop, tools, sessions, approvals | Customer identity, authenticated through the official runtime | Official runtime supervised by our Go worker |
| Customer inference endpoint | A declared, tested API dialect and model capability set | Customer infrastructure | Our Go loop and policy-controlled tools |

An agent CLI login does not turn subscription credentials into general API credentials. A customer-owned worker improves custody and isolation; it does not create permission that a provider has not granted.

## Support and entitlement matrix

“Supported” below means a documented technical integration path, subject to the customer's account agreement. “Conditional” means keep disabled until the named release gate passes. No row promises unlimited subscription usage.

| Provider/path | Hosted multi-tenant service | Self-hosted/customer worker | Initial decision |
| --- | --- | --- | --- |
| OpenAI API, customer key/workload credential | Supported | Supported | Native Go adapter; API billing |
| Codex official runtime, Business/Enterprise Codex access token | Conditional: verify deployment fits trusted automation and workspace terms | Supported for documented trusted local scripts, scheduled jobs, CI and app-server automation | First workspace-entitlement bridge candidate |
| Codex official runtime, managed ChatGPT login | Conditional: hosted service/account eligibility unresolved | Documented sign-in; autonomous deployment remains conditional on account/workflow eligibility | Keep credentials in customer worker; no extracted-token API adapter |
| Anthropic API/cloud provider credential | Supported | Supported | Native Go adapter; API/cloud billing |
| Unmodified Claude Code with end-user-owned sign-in | Conditional on official hosting conditions and intended automation | Conditional on account and workflow conditions | Separate official-binary bridge; follow provider flow |
| Claude Agent SDK offering our own subscription login | Requires prior provider approval | Same product-integration gate applies | API credentials by default |
| Gemini Developer API or supported Google Cloud API | Supported | Supported | Native Go adapter; customer project billing |
| Official Gemini CLI with Google-account login | Unverified for unattended multi-tenant subscription service | CLI sign-in/headless reuse documented; product automation eligibility conditional | Optional bridge after provider/deployment review |
| Gemini CLI OAuth used directly by our model client | Prohibited by documented service terms | Prohibited by documented service terms | Never implement |
| Customer Ollama/vLLM/compatible endpoint | Supported through explicit network and compatibility configuration | Supported | Probe capabilities; do not infer parity from protocol name |

Evidence for these classifications follows. API credential provisioning and commercial agreements must be checked for the actual customer deployment; the matrix is not a reseller authorization.

### OpenAI

Official OpenAI documentation distinguishes ChatGPT subscription sign-in from usage-based API-key access and says their governance/data controls differ. Preserve that distinction in the connection record and UI. [Authentication](https://learn.chatgpt.com/docs/auth)

Codex now documents Business/Enterprise access tokens for trusted non-interactive local workflows, including scheduled jobs, CI and app-server automation. Tokens are tied to workspace identities; the documentation explicitly warns against untrusted runners. This provides a stronger basis for customer-owned automation than merely observing that interactive login succeeds. It does not establish blanket authorization to pool seats or operate a third-party subscription service. [Codex access tokens](https://learn.chatgpt.com/docs/enterprise/access-tokens)

App-server provides a documented product integration surface: JSON-RPC, stdio JSONL, authentication, approvals and streamed agent events. Managed ChatGPT authentication leaves OAuth ownership with Codex; external-token mode is experimental. Use stdio initially and exclude external-token mode. TCP WebSocket transport is currently described as experimental/unsupported. [App-server](https://learn.chatgpt.com/docs/app-server)

The Codex SDK is intended for coding automation, CI and application integration. Current docs list TypeScript and Python libraries; a Go service can instead supervise the documented app-server protocol directly. There is no need to make the main backend TypeScript. [Codex SDK](https://learn.chatgpt.com/docs/codex-sdk)

Release gap: obtain deployment-specific confirmation before marketing hosted subscription consumption; validate supported workspace, identity ownership, entitlements, usage reporting and revocation with a real customer test account.

### Anthropic

Current legal documentation permits hosting the unmodified Claude Code binary under stated commercial conditions: retain built-in authentication choices, end users authenticate with their own accounts, usage bills directly to them, and do not resell/intermediate usage. It prohibits collecting or intermediating Claude.ai credentials/session tokens and distinguishes end-user sign-in through the official binary from offering Claude.ai login in a developer's own app. Pro/Max limits assume ordinary individual use. [Claude Code legal and compliance](https://code.claude.com/docs/en/legal-and-compliance)

The Agent SDK overview still requires prior approval for third-party products offering Claude.ai login/rate limits and directs developers to API authentication. It lists Python/TypeScript SDKs and a CLI subprocess path for other languages. Therefore official-binary hosting and an SDK-powered custom subscription login are separate release decisions. [Agent SDK overview](https://code.claude.com/docs/en/agent-sdk/overview)

The June 16 support page says the announced June 15 Agent SDK/headless billing change was paused: SDK, `claude -p`, and third-party usage still draw from subscription limits for now; the proposed monthly credit is unavailable. Its older pricing table is explicitly historical. Do not ship either “headless always requires API billing” or the withdrawn credit amounts as current product claims. This billing statement alone does not settle the product's authentication permissions. [Use the Agent SDK with your Claude plan](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan)

Release gap: reconcile the intended official-binary bridge, SDK usage, hosted scheduling, and user custody with Anthropic before enabling subscription workflows. Default direct integrations to customer API/cloud credentials. Current API authentication also documents workload identity federation, offering an eventual alternative to static API keys. [API authentication](https://platform.claude.com/docs/en/manage-claude/authentication)

### Google

Gemini CLI terms explicitly prohibit directly accessing its underlying services using third-party software with CLI OAuth credentials. Open-source CLI licensing does not override service terms. Never copy cached CLI tokens or its OAuth client identity into the Go API adapter. [Gemini CLI terms](https://geminicli.com/docs/resources/tos-privacy/)

The CLI documents Google-account sign-in and headless reuse of cached authentication; without existing credentials it directs headless users to API-key or cloud authentication. These are technical features, not evidence that an unattended hosted subscription product is authorized. Use direct API/cloud auth for general service operation and validate official-CLI bridges separately. [Gemini CLI authentication](https://geminicli.com/docs/get-started/authentication/)

Release gap: verify the actual Google plan, customer organization, deployment and automation pattern. Do not imply Google AI subscriptions fund arbitrary Gemini API calls. Native Go integration is available through Google's Gen AI SDK. [Gemini API libraries](https://ai.google.dev/gemini-api/docs/libraries)

### Self-hosted inference

Use explicit endpoint profiles, initially OpenAI Chat Completions plus independently enabled Responses support. Ollama documents compatibility and concrete limits, including stateless Responses behavior; do not enable server-side conversation features merely because an endpoint accepts `/v1/responses`. [Ollama compatibility](https://docs.ollama.com/api/openai-compatibility)

vLLM documents its OpenAI-compatible server and model-dependent configuration. Treat server version, model, template/parser configuration and supported operations as one deployment profile. Model licensing, GPU capacity, latency and output quality remain customer/deployment checks. [vLLM server](https://docs.vllm.ai/en/latest/serving/online_serving/openai_compatible_server/)

## Go integration contract

Use two small interfaces, not a universal agent framework. Direct providers generate events and request tools; agent runtimes execute their own loop and expose approvals. Both feed the same durable run record.

The names below describe research semantics. The selected names and boundary are `ModelProvider` and `AgentExecutor` in [contracts.md](../contracts.md); that document is authoritative for T01.

| Contract element | Required semantics |
| --- | --- |
| `ModelProvider.Describe` | Return connection-specific capabilities and limits, not promises inferred from model-name prefixes |
| `ModelProvider.Stream(ctx, request, emit)` | Normalized message/tool/usage/terminal events; callback backpressure and context cancellation; no tool execution inside adapter |
| `AgentRuntime.Start / Resume / Cancel` | Launch/resume the pinned official runtime; map lifecycle and approvals; refuse unsupported policy requirements |
| `AgentRuntime.DecideApproval` | Apply an already-recorded policy/human decision to one scoped request; never infer approval from silence |
| Request identity | Tenant, connection, run, attempt, repository revision, policy version, model/version, budget reservation |
| Event identity | Monotonic sequence plus provider response/item/call IDs; duplicates are detectable |
| Terminal outcome | Completed, canceled, quota exhausted, authentication required, policy denied, failed or interrupted/unknown; EOF alone is not success |
| Usage | Input/output/cache/reasoning counts when available; provider currency cost or estimate explicitly labeled; unknown is never zero |
| Opaque state | Provider-owned continuation/reasoning metadata stored tenant-scoped; never flatten into text or replay across providers |

Use official Go API clients where practical: OpenAI and Anthropic document Go libraries, independently of their agent SDK language choices. Pin dependency versions and keep provider structs inside adapters. [OpenAI SDKs](https://developers.openai.com/api/docs/libraries), [Anthropic SDKs](https://platform.claude.com/docs/en/cli-sdks-libraries/overview)

Capability records must include tool calling, parallel calls, structured output/schema limits, streaming, cancellation, usage availability, context/output limits, image input if used, continuation support, and runtime approval/policy interception. Store tested provider/runtime versions and last verification time. Offer role presets such as planning, editing and review; administrators select concrete eligible model IDs. Snapshot the resolved ID on each attempt. A stronger model never receives broader permissions automatically.

### Streaming and tool safety

Accumulate tool arguments by call/item identity until the provider marks them complete. OpenAI streams argument deltas and final call items; Anthropic streams indexed content blocks with partial JSON. Their event order and completion semantics differ. Keep separate parsers. [OpenAI function calling](https://developers.openai.com/api/docs/guides/function-calling), [Anthropic streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)

Gemini adapters must preserve the full relevant provider response state. GenerateContent has documented thought-signature replay requirements; current Interactions and GenerateContent surfaces differ. Select and pin one supported surface per adapter version, validate Go SDK coverage, and keep API-specific state opaque. [Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling), [GenerateContent thought signatures](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures)

Before executing any requested tool: validate the complete JSON against the registered schema, enforce size/path/argument limits, resolve policy, then record a durable execution claim. Never execute partial streamed arguments. Tools write results to a ledger keyed by run/attempt/call ID; external mutations additionally require operation-level deduplication across retries and provider changes. Serialize conflicting filesystem actions. Retry generation only when the previous attempt's effects are known; ambiguous interruptions pause for reconciliation.

Provider “strict JSON” and agent approval hooks are helpful inputs, not security boundaries. Unknown required event shapes fail closed. Harmless unknown telemetry may be ignored with versioned diagnostics. Drain subprocess stderr separately, bound stream buffers, terminate process groups on cancellation, and clean temporary workspaces after artifacts are captured.

## Official bridges without changing the Go backend

Start with Go supervising child processes over stdio. Codex uses app-server; Claude supports `-p` with JSON/JSONL output; Gemini headless mode documents JSONL lifecycle, message, tool and result events. A reported tool event may describe execution already performed by the runtime; do not treat every event as a pre-execution approval request. [Claude programmatic operation](https://code.claude.com/docs/en/headless), [Gemini headless reference](https://geminicli.com/docs/cli/headless/)

If required permission callbacks or session controls are available only through an official SDK, use a narrow, optional TypeScript/Python sidecar behind the same local contract. Its only duties are runtime lifecycle, event translation and approval transport. Keep scheduling, tenant authorization, budgets and publication in Go. Pin runtime/sidecar versions and image digests. Do not fork binaries to suppress authentication choices or manufacture subscription eligibility.

A customer worker can poll outbound for signed, tenant-scoped jobs. Keep official runtime authentication and provider secrets on that worker; return artifacts, policy results and redacted events. Expose neither a general remote shell nor an arbitrary process-launch endpoint. A hosted agent bridge needs equivalent per-customer isolation and a deployment-specific entitlement decision.

The spike must prove tool-policy interception before enabling mutation. If a runtime cannot enforce a required boundary, limit it to read-only planning or refuse that workflow. The publish service independently validates artifacts, policy and repository revision before any external write; the agent never holds the publishing credential.

## Secrets, isolation and spend

Each tenant connection records credential kind, account/project owner, billing mode, allowed deployments, allowed models, approved destination and evidence date. Store API secrets through envelope encryption or a customer vault reference, decrypt only for the authorized process, and keep credentials out of prompts, browser responses, job payloads and logs. Official runtime session stores remain private to the customer identity; never share login directories across tenants.

Execute repository code in a separate sandbox from the process holding model credentials. A sanitized environment alone is insufficient if tests can read same-user processes or mounted auth directories. Use a narrowly scoped model gateway/official execution separation where supported, or a dedicated trusted worker; do not advertise hostile-repository isolation until verified. No host mounts, Docker socket, cloud metadata or unrelated network access. Repository instructions, hooks and downloaded tools are untrusted inputs and cannot change credentials, budgets or publication policy.

Custom endpoints need an administrator-owned allowlist, TLS/CA validation and redirect restrictions. Hosted workers must reject arbitrary private/loopback/link-local destinations and DNS rebinding; explicit customer-network access belongs on a customer worker or approved private connection. Never forward a provider secret to a newly selected base URL.

Prefer a configured eligible subscription connection, but quota exhaustion pauses or queues the run by default. API fallback, provider changes and paid overage require a persisted administrator opt-in naming destination, billing account, data residency and spending caps. Switching providers also requires approval to send repository data there. Retry budgets, concurrency and wall-clock limits apply to subscription and API runs alike. Reserve spend before calls, reconcile actual usage afterward, and block new work when usage is unknown beyond the allowed reserve. Subscription usage is quota consumption, not “free tokens.”

## Release gates

1. **Provider eligibility:** dated source review plus customer/deployment confirmation for every subscription bridge; unresolved paths remain disabled. Test login, logout, revocation, quota exhaustion and billing identity. Recheck when terms or runtime versions change.
2. **Adapter correctness:** contract fixtures and sandbox integration runs cover split/malformed arguments, multiple tool calls, continuation metadata, interrupted streams, late usage, cancellation and unknown events. No production mutation during qualification.
3. **Policy containment:** attempt repository prompt injection, malicious hooks, secret reads, forbidden network access, symlink/path escape and cross-tenant access. Runtime must deny before effect; publication stays independently gated.
4. **Cost behavior:** show selected billing source before execution; exhaust subscription quota and verify no unapproved API spend. Verify retry/failover cannot duplicate an external mutation.
5. **Deployment parity:** prove native APIs and customer inference endpoints in both hosted and self-hosted modes; qualify customer workers separately from hosted agent bridges. Ship an explicit tested support matrix.

No credentials were used and no live integration or entitlement test was performed for this research. Confidence: 90% in the documented distinctions and implementation direction; hosted subscription eligibility remains an explicit validation gap.
