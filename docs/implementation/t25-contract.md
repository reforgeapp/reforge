# T25 implementation contract

Status: architecture frozen for local implementation; acceptance pending.

Campaigns use existing repair, native pipeline and protected GitOps services. They never supply an alternative native approval or execution path. One campaign has one kind. A repair campaign targets validated publication or canonical merged outcomes; pipeline/GitOps campaigns require attributed healthy deployment. Recovery, unknown accounting, failed/unverified canaries and changed authority stop expansion.

Selection is an explicit snapshot of up to1,000 repository members. The original filter description is retained only as provenance; no saved-filter reevaluation grows membership. Start pins recipe/image, finding evidence, relevant connection/configuration versions, effective policy hashes, model route, canary groups, concurrency, stage sizes, success criterion, observation window, thresholds and maintenance windows. Runtime preflight still revalidates native heads, credentials, budgets and permissions. Changed pins require a new reviewed campaign and renewed canaries.

Preview uses current persisted metadata and scoped policy resolution; it is an eligibility snapshot, not a successful execution claim. Native preflight occurs immediately before each dispatch through existing services. Missing evidence remains an excluded member in the denominator. Automatic canaries represent each recipe/forge-version/validation group; explicit canaries must cover the same groups. Pipeline/GitOps configurations must observe health for at least the campaign window. UTC weekly windows use weekday0=Sunday and inclusive start/exclusive end minutes; split overnight windows explicitly.

Controller dispatches at most one new action per claimed campaign turn, rotates organisations and campaigns, and releases database locks before provider I/O. Persistent ownership/version checks and stable member operation keys reconcile lost responses. Each admission and resumed action rechecks the initiating identity. Pause propagates to pending tasks and requests qualified native cancellation; already completed mutations stay visible. Resume requires a reason and explicit continuation of the current stage; failed/changed canaries require a renewed campaign. No automatic rollback or cross-repository atomicity claim.

HTTP under `/api/v1/orgs/{orgID}`:

- `POST /campaign-previews`: `campaign.Input` → `campaign.Preview`. Member requests use existing repair/pipeline/GitOps request types. Required kind=`repair|pipeline|gitops`, success=`published|merged|healthy`, canary_size1–100, batch_size1–100, concurrency1–100, observation_seconds0–86400, failure_limit0–1000, failure_percent0–100. Exceeding either failure threshold stops expansion; any canary failure always stops it.
- `POST /campaigns`: `{preview_id,idempotency_key}` → Campaign (planned).
- `GET /campaigns?state&cursor&limit`: standard complete/next_cursor page.
- `GET /campaigns/{id}` → Campaign; `GET /campaigns/{id}/members?cursor&limit` → paged Member.
- `POST /campaigns/{id}/start|pause|resume|cancel`: If-Match, CSRF, `{reason,stage_decision?}` → Campaign. Resume requires `stage_decision:"continue_current_stage"`.

Read access requires all source/delivery scopes in the snapshot. Mutations require current maintain/manage permissions on all member repositories. Repair campaigns require a configured campaign budget; budget setup uses existing owner-only budget APIs. GUI exposes exact exclusions, counts, canary groups, pinned inputs, budget blockers, lifecycle evidence and ordinary run/deployment links.

Frozen Go/JSON types: `internal/campaign/types.go`. Coordinator owns schema, API generation, authority and controller. GUI worker owns only new campaign API/page files until reviewed.

Scheduled actions use a non-serializable internal automation grant, not a manufactured browser session. The grant is limited to the frozen repositories and maintainer actions; every transaction rechecks current originator membership, repository access and the campaign authorization record. Browser logout does not erase the user's explicit campaign approval. Pause/cancel, changed permissions and grant expiry prevent further dispatch. Grants last at most30days and require explicit user review to renew. Automation grants cannot create or control campaigns.
