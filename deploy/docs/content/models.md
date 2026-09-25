# Model routes

Model setup starts by choosing a connection type:

- **Provider API** — OpenAI, Anthropic or Google.
- **Self-hosted compatible endpoint** — URL, protocol profile, model, assigned runner and
  network path, and auth.
- **Official agent on your runner** — see [Agent runtimes](agents.md).

## Capability probe

The probe records tools, structured output, streaming and usage support, the supported
recipe roles and known limitations. An unsupported or unverified model stays disabled for
the affected role.

## Billing routes

Set a model's price in **Connections → Models & agents → (model) → Pricing**: input and
output dollars per million tokens. Saving creates the `default` priced route with limits
of 200,000 input tokens, 32,000 output tokens, 10 minutes and one request per turn.

Every route records whether it is priced API usage or subscription quota. Pricing uses
operator-pinned conservative rates; the estimate is distinct from provider invoice
reconciliation. Unknown usage is held, not zeroed.

Subscription statuses are eligible, verification required, quota unknown, exhausted or
disconnected. A route never automatically falls back to a paid API, and a login is never
treated as entitlement.

## Self-hosted compatible endpoints

The compatible profile speaks the OpenAI Chat Completions protocol. A Responses option is
probed independently. Malformed or unsupported tool output fails safely rather than
executing partial arguments.
