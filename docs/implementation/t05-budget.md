# T05 budget integration

`internal/budget` reserves integer microUSD, tokens, milliseconds, requests and concurrent allowances. One microUSD is USD 0.000001. No exchange rates or live provider calls are used. Install root migrations 006–008; `internal/budget/schema.sql` describes the budget tables and workflow migration 008 supplies pinned model routes.

## HTTP configuration

Construct `budget.New(store, auth, fenceCheck, campaignCheck)`. `FenceCheck(ctx,tx,budget.Lease)` must call the trusted workflow validator under the same transaction; nil denies reservation/dispatch. `CampaignCheck(ctx,tx,orgID,campaignID)` must resolve a real authorised campaign; nil disables campaign configuration/dispatch until T25. Existing session/CSRF middleware applies. Owner alone writes limits/routes; owners view all scopes, repository readers may view their repository budget and reservation records.

| HTTP shape | Service |
| --- | --- |
| `GET /api/v1/orgs/:orgID/budgets/:kind/:scopeID` | `GetLimit(ctx,session,orgID,Scope)` |
| `PUT /api/v1/orgs/:orgID/budgets/:kind/:scopeID`, body `Limit`, quoted `If-Match` | `PutLimit(ctx,session,orgID,limit,expected,requestID)` |
| `GET /api/v1/orgs/:orgID/budget-routes/:connectionID?model=…&route=…` | `GetRoute(ctx,session,orgID,connectionID,model,route)` |
| `PUT /api/v1/orgs/:orgID/budget-routes/:connectionID`, body `Route`, quoted `If-Match` | `PutRoute(ctx,session,orgID,route,expected,requestID)` |
| `GET /api/v1/orgs/:orgID/budget-reservations/:reservationID` | `Get(ctx,session,orgID,reservationID)` |

Derive IDs from route parameters; return the resulting version as ETag. Scope kinds: `organisation`, `team`, `repository`, `campaign`, `connection`. Campaign configuration requires the trusted existence callback, and only the server-persisted workflow task selects that campaign during accounting. Root owns transport/OpenAPI wiring. Capacity/unknown/revoked responses block dispatch; conflicts require refreshed versions. No HTTP endpoint accepts reservation, settlement, qualification or cancellation proof.

Periods are UTC `daily`, UTC `monthly`, or fixed `custom`. Daily/monthly require unset start/end; custom requires explicit increasing timestamps and denies dispatch outside that interval. Period/start/end cannot change after creation, so ceiling edits cannot erase spend. Outstanding holds are stored separately from period spend and remain counted across every rollover. Final charges use the original reservation period; unknown holds are never reset by the calendar. Concurrency tracks outstanding reservations, including uncertain operations, and releases only on definitive settlement/cancellation.

## Workflow/controller contract

All controller methods accept an existing tenant `pgx.Tx`; any error must roll back the transaction. Lock order is organisation → job/attempt → reservation/accounts. The injected callback composes `workflow.ValidateFenceTx` with reservation/check/dispatch in the same transaction. Workflow checks current policy and recipe/campaign/pool pauses; budget independently checks live DB-clock lease/fence/worker/task, organisation/repository state, model connection and budget/route versions.

1. Build `budget.Lease` from the trusted workflow lease. Its fields match workflow's lease. Assign a stable UUID `Quote.OperationID` per bounded inference allowance; retries of the same allowance reuse it. A new genuinely separate attempt needs a new UUID and full capacity. Quote limits describe total input/output tokens, duration and requests across the entire allowance, not per request.
2. Call `ReserveTx(ctx,tx,lease,quote)`. Model connection/campaign/route come from the persisted task, and all repository teams come from current bindings. The quote route must equal the pinned task route; the model must match current connection configuration. Org budget is mandatory; a task campaign requires its budget. Configured team/repository/connection budgets all apply; absent child budgets inherit. No caller can omit an ancestor. Policy route identifiers use `connectionID/routeName`; connection billing kinds `direct_api` and `subscription` remain separate invariants.
3. Before the external call, call `MarkDispatchedTx(ctx,tx,lease,reservationID)` with the workflow outbox intent/fence validation. Commit before sending. It rechecks current scopes, window/ceiling versions, route, connection and live lease. Existing `dispatched`, `unknown`, `settled` or `cancelled` reservations cannot dispatch again. `CheckTx` performs the same check without changing state.
4. On timeout, call `MarkUnknownTx` or `SettleTx` with `Known=false` and a reference. The full hold remains; reconcilers must query the original provider operation before retrying.
5. Only trusted, authenticated provider/controller evidence may call `SettleTx(...,Settlement{Known:true,Actual,Reference})`. Runner/browser-supplied totals are not proof. Actual concurrency must be zero. Settlement is idempotent for exactly the same final amount/reference. Unknown reports cannot replace final usage.
6. `CancelTx` releases a never-dispatched reservation directly. After dispatch/uncertainty it requires trusted definitive evidence that the provider incurred no usage; lease expiry or user cancellation alone is insufficient.

Authoritative usage above the reserved maximum is recorded in full, with `Reservation.Debt`. Every affected budget is persistently paused/versioned and a `budget.paused` audit record is emitted. Old-period overruns therefore block new calls after rollover. Late definitive charges after a previously confirmed-unused cancellation are accounted as debt. Owner reconciliation and explicit unpause are required; the service never clamps observed cost to the estimate. Final reconciliation remains valid after worker lease expiration or connection revocation.

## Pricing and subscription routes

`priced` requires an explicit pricing version, non-negative input/output microUSD-per-million rates, per-request overhead ceiling, positive token/time maxima and exactly one request. Every direct API retry therefore needs a separate budgeted operation. Org requires monetary and concurrency caps. Quote cost rounds upward with overflow-checked integer arithmetic; provider-specific additional charges must be covered by the request overhead ceiling or the route must remain unavailable. Adapters must enforce the reserved output/token/time/request envelope before execution. Reservations retain the exact pricing and route snapshot; route changes invalidate undispatched reservations. A verified unmetered self-hosted model can use an explicitly versioned zero-price route; it is distinct from an unpriced API.

`quota` is restricted to an official subscription agent connection and requires explicit org token/time/request/concurrency caps. Its zero microUSD value means currency usage is unmetered, not a verified free API price. Browser configuration cannot claim qualification. T16 calls `QualifyRouteTx` only after proving account/topology entitlement, credential isolation and enforceable limits; the transition records `budget.route_qualified` with its evidence reference. Changing route configuration invalidates qualification. No implicit subscription-to-paid fallback exists.

## Validation

`go test ./internal/budget` runs pricing/window/overflow checks. Set `REFORGE_TEST_DATABASE_URL` to the guarded disposable `reforge_test` database for PostgreSQL tests. The contention test runs 100 reservations against six derived scopes; exactly the tightest ceiling succeeds. Tests also cover restart/idempotency, changed quote rejection, unknown usage across rollover, cancellation without proof, exact-once settlement, persistent overrun debt, stale fences, ceiling changes, connection revocation, missing pricing and subscription qualification.
