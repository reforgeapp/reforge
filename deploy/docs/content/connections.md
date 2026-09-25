# Connections

**Connections** has three groups: forges, models and agents, and delivery integrations.

## Forges

Create a forge connection from **Forges** with **Add connection**. Choose the
provider, authenticate, then preview and import repositories. Cloud API addresses
are prefilled; **Advanced** holds an optional namespace, the API address and a CA
certificate.

### GitHub

GitHub offers a guided App installation, a personal access token, and a manual App.

GitHub namespace: blank (token owner's repositories), `org` or `org:org`, `user:name`, or
`owner/repo` (or its URL) for a single repository.

The guided App flow is the default. Select **Set up GitHub App** (self-hosted) or
**Connect GitHub** (hosted) and follow the handoff to GitHub:

1. Reforge starts a short-lived setup and opens GitHub.
2. On GitHub, review the requested permissions and create or install the App.
3. GitHub returns to Reforge. Reforge confirms the signed-in user owns or
   administers the target account, records the connection, then opens the
   repository selection step.
4. Choose repositories and select **Import selected**.

Self-hosted Reforge registers the App from a manifest; the hosted edition installs
an App the operator configured. Both editions use the same explicit ownership check:
the signed-in user must own the personal target or be an active administrator of the
target organisation. Reforge verifies the installation belongs to the configured App
and is not suspended before it records a connection.

If GitHub creates the App but Reforge cannot finish, no connection is saved. Use the
management link in the error to delete or reconfigure the App under your GitHub
account settings, then start again.

For a personal access token instead, choose **Personal access token** under
**Authentication**, paste the token and select **Create connection**. The token
needs read access to repositories (GitHub `repo`, GitLab `read_api`, Gitea `repo`);
the **Get token** link beside the field opens the provider page, using the entered
instance host for self-managed Gitea and GitLab. Reforge tests the connection and
opens the repository selection step; choose repositories and select **Import
selected**. A failed test keeps the connection: paste a corrected token and select
**Update credentials**, or use **Retry test** when the token was approved elsewhere
(for example a GitHub SSO authorisation). Both act on the same connection and never
create another. A test that returns an unhealthy state does not open the repository
step; read the reason and retry after fixing it.

A GitHub token connection is read-only for Reforge: fixes are published only through the
GitHub App, so its bot authors every branch and pull request Reforge owns.

#### GitHub Enterprise Server and manual App

GHES keeps the manual App flow. Under **Authentication**, choose **GitHub App
(manual)**, then provide the **App ID**, **Installation ID** and the App **RSA
private key**. Upload the `.pem` file under **Private key file** or paste it into the
**Private key PEM** field. Newlines are preserved and the key is sent only with the
create request. Credentials are write-only and are never returned to the browser.

### GitLab and Gitea

GitLab uses a token; self-managed instances set the endpoint and CA. Gitea uses a
token and a distinct **instance URL** for the server. Do not enter a repository URL
as the API address: a repository URL belongs in the optional repository filter, and
Reforge rejects a repository URL in the API-address field with an actionable message
instead of saving an invalid endpoint.

### Repository import

Creating a forge connection runs the capability test automatically. When it is
healthy Reforge opens **Sync forge inventory** and starts the preview; the preview
lists candidate repositories with checkboxes. Select repositories and choose
**Import selected**, or **Import all**. A failed test keeps the connection and shows
**Retry test** and **Update credentials**; retry on the same connection instead of
creating a duplicate. Imported repositories appear under **Repositories**. If a
repository filter is set, only the matching visible repositories are imported.

## Provider detail

A forge detail view shows the provider, API address, authentication kind, last
checked time and status, with an **Add repositories** action. Model, runtime and
billing fields belong to model connections and are not shown for forges. Advanced
CA and private-runner settings are secondary. A revoked connection disables **Test
connection** and **Revoke** and shows the reason.

**Edit** changes the name and, for token forges, the namespace; the next sync uses the
new namespace. **Delete** removes a connection and its stored credential when no
repository, task or billing route uses it; otherwise revoke it.

## Development and webhook limitation

Production requires an HTTPS `REFORGE_PUBLIC_URL`. On a loopback development server
the guided manifest is created with its webhook inactive, and the connection reports
a truthful pending webhook status. To receive events, set a public HTTPS
`REFORGE_PUBLIC_URL`, update the webhook URL and secret in the GitHub App settings,
then recreate or reconfigure the App. Reforge does not create tunnels or hidden
callbacks. Manual token connections are unaffected.

## Hosted App configuration (operators)

The hosted edition installs one shared GitHub App. It requires
`REFORGE_EDITION=hosted` and the hosted KMS key material (`REFORGE_KMS_REGION`,
`REFORGE_KMS_KEY_ARN`); see [Install](install.md) and
[Backup and restore](backup-restore.md). The overlay below applies only to that
hosted deployment: the default self-hosted stack registers its own App from a
manifest and does not use it.

Set all six variables together or the server fails startup validation:

- `REFORGE_GITHUB_APP_ID`, `REFORGE_GITHUB_APP_SLUG`, `REFORGE_GITHUB_APP_CLIENT_ID`
- `REFORGE_GITHUB_APP_CLIENT_SECRET_FILE`, `REFORGE_GITHUB_APP_PRIVATE_KEY_FILE`,
  `REFORGE_GITHUB_APP_WEBHOOK_SECRET_FILE`

The base Compose file does not pass these variables to the server. Apply the
optional overlay, which mounts the three secret files read-only and wires the six
variables:

```sh
GITHUB_APP_CLIENT_SECRET_HOST_FILE=/etc/reforge/github-app-client-secret \
GITHUB_APP_PRIVATE_KEY_HOST_FILE=/etc/reforge/github-app-private-key.pem \
GITHUB_APP_WEBHOOK_SECRET_HOST_FILE=/etc/reforge/github-app-webhook-secret \
docker compose -f compose.yaml -f github-app.override.yaml up -d
```

The server container runs as UID 10001 and reads the mounted files as that user.
Make each host file readable by it, for example `chown 10001` and mode `0400`, or a
controlled group with mode `0440`. The files stay on the host and are never baked
into an image or sent to the browser.

Create the App in GitHub with:

- **Homepage URL** `${REFORGE_PUBLIC_URL}`.
- **Callback URL** `${REFORGE_PUBLIC_URL}/auth/github/oauth/callback`.
- **Setup URL** `${REFORGE_PUBLIC_URL}/auth/github/install/callback`, with **Redirect on update** enabled so reconfiguration returns to Reforge.
- **Webhook URL** `${REFORGE_PUBLIC_URL}/hooks/github/app`, using the same secret as `REFORGE_GITHUB_APP_WEBHOOK_SECRET_FILE`.
- **Request user authorization (OAuth) during installation** left off; Reforge runs its own PKCE authorisation after the install callback.
- Repository permissions: metadata read, contents write, pull requests write, checks write, commit statuses read, actions write, administration read, and organisation members read (used to confirm the signed-in user administers the target organisation).
- Events: push, pull request, repository, check run, check suite, status and workflow run.

The identifier values are public. The client secret, private key and webhook secret
are write-only and are never returned by the API or shown in the browser. Self-hosted
deployments do not need this configuration; their manifest requests the same
permissions and events.

## Troubleshooting

- **Guided setup unavailable**: the deployment lacks a public HTTPS URL, or an
  operator has not configured the hosted App. Use a personal access token, or fix
  the deployment as the reason states.
- **Setup in progress**: a previous attempt is unfinished. Resume it, or cancel and
  start again.
- **Setup failed or expired**: no connection was saved. Delete or reconfigure the
  App under the GitHub account settings linked in the error, then start again.
- **Capability test failed**: the connection is kept. Fix the token or App access and
  select **Update credentials**, or **Retry test** if it was approved elsewhere; do
  not create another.
- **No repositories listed**: confirm the connection is healthy and the token has
  repository read access, then run the preview again.

## Model providers

Create a model connection from **Models & agents** with **Add connection**:

1. Keep **Connection type** as **Model API** (choose **Agent runtime** for an agent) and choose a **Provider**.
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
gateway is shown as **Key untested** until a model call through it succeeds, then as
**Key verified**. A catalogue entry the provider marks
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
