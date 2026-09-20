# T01 contract ownership

The coordinator owns `api/openapi.yaml`, all manifests/lockfiles, `internal/domain`, provider `contracts.go` files, generated output and migration numbering. Workers request changes; they do not edit these paths.

Go boundaries: `internal/forge/contracts.go`, `internal/model/contracts.go`, `internal/agent/contracts.go`, `internal/sandbox/contracts.go`, `internal/artifact/contracts.go`. Stateless model cancellation uses the `StreamTurn` context; no process-global cancel method. Provider instances are created for one validated tenant connection and injected with its destination-validated HTTP client. Secrets never enter serialised adapter records.

Generator inputs: OpenAPI 3.0 JSON (valid YAML) in `api/openapi.yaml`; SQL in `internal/store/migrations` and `internal/store/queries`. `make generate` pins oapi-codegen 2.8.0, sqlc 1.31.1 and openapi-typescript 7.13.0. SQL name directives are executable generator syntax. Generated explanatory comments are stripped. `make check` rejects generation drift.

T02 API contract: `GET /api/v1/session` returns user, organisations, memberships and CSRF token. Unauthenticated requests return structured 401. Browser mutations require `X-CSRF-Token` plus same-origin checks; cookies stay HttpOnly. Org APIs use `/api/v1/orgs/:orgID`. Sessions and organisation selection never grant implicit repository access.

T04 shell receives session from that endpoint, resets queries on org switch, uses `/org/:orgID/:section` deep links, and exposes server-provided development/fixture status. T17 and later routes must use real endpoints. The initial shell does not populate business records with prototype data.

New SQL migrations are additive and immutable after application. Migration owner credentials never enter the server or runner. PostgreSQL runtime transactions set org/user context locally; pooled sessions must not retain it.
