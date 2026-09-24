# Connections

**Connections** has three groups: forges, models and agents, and delivery integrations.

## Forges

- GitHub uses an App installation or a token connection.
- GitLab uses a token; self-managed instances set the endpoint and CA.
- Gitea uses a token and supports scoped identity and OpenAPI capability discovery.

After creating a connection, run the capability probe. The probe records the server
version, scopes and the features that are actually available. A missing feature disables
only the affected action and shows the reason.

## Model providers

Create a model connection from **Models & agents** with **Add connection**:

1. Set **Kind** to **Model API** and choose a **Provider**.
2. Enter the provider **API key**.
3. Select **Test connection** to load the provider's model catalogue.
4. Choose a **Model** from the returned list.
5. Select **Save connection**. Reforge creates the connection, then probes the selected
   model to record its capability state.

Built-in providers are OpenAI, Claude (Anthropic), Gemini (Google), OpenCode Zen and
OpenCode Go. OpenAI, Anthropic and Google use their native protocols; the OpenCode
gateways use a compatible profile — see the official
[OpenCode Zen](https://opencode.ai/docs/zen/) and
[OpenCode Go](https://opencode.ai/docs/go/) documentation. **Custom / compatible** covers
an existing OpenAI-compatible endpoint such as Ollama or vLLM and shows the endpoint in the
form; its protocol profile is under **Advanced**.

The API key is an inference credential for this provider. The browser keeps it in memory
only for the test and the create request and never writes it to browser storage; after
**Save connection** the server stores it encrypted and never returns it to the browser.
This form connects with a provider API key. Agent runtimes use their own official login and
are configured separately (see [Agent runtimes](agents.md)).

**Advanced** holds the connection name (generated from the provider and model by default),
the endpoint, a protocol profile for compatible endpoints, an optional CA certificate and a
manual **Model ID**. Use a manual model ID when the provider has no public catalogue or
when the endpoint is reached over a private route.

The catalogue lists the account- and key-scoped models for native providers. The OpenCode
catalogue is public and does not verify the key or exercise inference, so a saved OpenCode
gateway is shown as catalog only rather than verified. A catalogue entry the provider marks
unavailable is disabled and cannot be saved, and an unrecognised OpenCode model family stays
disabled with its reason. If the post-create probe fails, the connection is retained but its
state is unknown until **Retry verification** reads the latest state; retry on the same
connection instead of creating another.

## Credentials

Credentials are write-only. The interface shows a fingerprint or last characters where
the provider is safe, the rotation date and a revoke control. Rotation and revocation are
immediate. Connection failures distinguish authentication, permission, reachability,
incompatible protocol and exhausted quota.

## Private routes

A private forge or model endpoint is reachable only through an approved route: a fixed
host and CIDR set pinned to an enrolled runner. Redirects, DNS changes and metadata
destinations cannot expand the scope.

## Delivery

Delivery connections configure native pipeline access or a GitOps delivery repository.
See [Deployments](delivery.md).
