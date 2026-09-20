# T17.1 connections and runners UI

The administration routes use the session-scoped API client for forge, model, agent and delivery connections plus runner pools and enrolled runners. Query keys include the organisation, and mutations carry the current CSRF token and `If-Match` version so stale edits remain actionable server errors.

Connection forms accept the frozen provider settings and exact model profiles: OpenAI Responses, Anthropic Messages, Google Gemini, and compatible Chat Completions, Ollama, vLLM or Responses. Secrets are sent only on create or rotation, cleared from component state after success, and never persisted to browser storage. Subscription billing remains visibly selectable but the backend response controls whether it is enabled.

Private routes require a runner ID, hostname and explicit CIDRs. The UI reports backend errors such as missing enrolled runners without inventing approval. Runner pools support empty onboarding pools, explicit repository IDs, state changes, enrollment token issuance and runner revocation. Enrollment tokens are held only in memory and presented once with user secret-file launch guidance.

Checks use the real generated client contract and cover loading, empty, error, stale-version and capability-unknown states through the shared state panels. Browser qualification against a live enrolled private runner remains pending an enrolled private-runner fixture; no fixture records are presented as successful backend data.

Live browser evidence on 2026-09-20: the non-fixture scenarios in `tests/connections.spec.ts` passed 3/3 against `http://127.0.0.1:8080` with the development session. The live scenario passed with an enrolled private runner and disposable bot-owned Gitea repository: it created and tested the private connection, ran repository preview, selected only the new repository, imported it, verified inventory freshness in the detail dialog, rotated and retested the credential, then revoked the connection. The disposable repository was deleted in test cleanup. Trace capture remained off; no credential was printed or retained. Typecheck and production build passed.
