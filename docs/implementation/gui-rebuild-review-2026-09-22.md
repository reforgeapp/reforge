# GUI rebuild review — 2026-09-22

## Current authority

GUI rebuild locally verified (agent-reviewed). T29 acceptance remains open for documented policy workflow gaps: automatic stored-evidence/portfolio simulation. The approved direction is a restrained operator console: white workspace, light slate navigation, blue controls, 14px body text, flat inventory surfaces, functioning tabs/workspaces and persisted records. See [GUI policy requirements](../design/gui.md). Earlier “ground-up complete” claims remain historical.

Current evidence is local and bounded: `go test ./...` and `make check` pass; browser suite `.local/rebuild-browser-final.json` records 108 passed, 0 failed, 9 opt-in skipped; fresh policy/insights checks pass in `.local/policy-final.json`; persisted policy activation, zero budget and audit export pass in `.local/live-controls.json`; 26 persisted demo routes pass with zero page errors, overflow or axe violations in `.local/rebuild-final/review.json`; visual opt-in checks pass in `.local/visual-final.json`; eight persisted detail focus-restore checks pass in `.local/rebuild-final/detail-review.json`. Final control build passes in `.local/rebuild-container-final.log` with image `sha256:c7a4a8ff32ddec50267ee97cfc3d55617582cfc621fb88fe2096f82ce173d6f2`. Served assets are `/assets/index-D-UESMP2.js` and `/assets/index-DfwrJMgn.css`. No human approval, G5 completion or external certification is claimed.

Demo organisation `00000000-0000-4000-8000-0000000000de` uses honest unverified `*.invalid` endpoints. Empty Runs, Changes and Deployments are persisted demo state; fixture-only coverage remains separate. Review captures identify the served build, browser and cache reset.

## Local review stack

Launch the review stack from the repository after stopping only the exact services listed
below. Reuse the existing `.local/development.env` credentials:

```sh
/tmp/reforge-postgres/bin/pg_ctl -D .local/postgres -l .local/postgres-review.log -o '-h 127.0.0.1 -p 55432 -k /home/mnorris/repos/reforge/.local/pgsocket' start
set -a; . .local/development.env; set +a
export REFORGE_MODE=development REFORGE_FIXTURE_AUTH=true REFORGE_EDITION=self-hosted REFORGE_ADDRESS=127.0.0.1:8080 REFORGE_PUBLIC_URL=http://127.0.0.1:8080 REFORGE_DOCS_URL=http://127.0.0.1:8082/docs/
setsid --fork ./bin/reforge >.local/server-review.log 2>&1 </dev/null
docker start reforge-review-docs
```

Open `http://127.0.0.1:8080` (docs at `http://127.0.0.1:8082`; direct demo organisation
URL: `http://127.0.0.1:8080/org/00000000-0000-4000-8000-0000000000de/overview`). The
captured demo organisation is `00000000-0000-4000-8000-0000000000de`.

PostgreSQL runs on `127.0.0.1:55432` (PID `2952266`), application on
`127.0.0.1:8080` (PID `2976298`), and docs container `reforge-review-docs` on
`127.0.0.1:8082`. Verify PID command lines before stopping; use `kill 2976298`,
`/tmp/reforge-postgres/bin/pg_ctl -D .local/postgres stop -m fast`, and
`docker stop reforge-review-docs` respectively. Do not use broad process matching.

Material changes reviewed: light navigation and white workspace, blue controls, flat
14px inventory surfaces, shared tabs/workspaces, persisted list/detail context, named
selectors and explicit loading/error/unknown states. Temporal, AWX and Rundeck inform
workflow hierarchy only; no external certification is inferred.

Policy workflow decision: presets may alter a draft only under the required RBAC, CSRF,
reason, immutable-save, simulation-hash and CAS-activation checks. Delivery presets retain
explicit allowlists; a null allowlist remains deny-all until the operator chooses one.
Portfolio simulation will use persisted repository, finding and change APIs with the
existing server evaluator, scope filtering, pagination and explicit partial-coverage
reporting. Missing native approvals, branch protection or attestations cannot be inferred
from simple checks: unknown blockers remain blocking, and preview never activates.

| Check | Result | Evidence |
| --- | --- | --- |
| Go and repository checks | Pass | `go test ./...`, `make check` |
| Browser suite | 108 passed, 0 failed, 9 opt-in skipped | `.local/rebuild-browser-final.json` |
| Policy editor fixture checks | Pass | `.local/policy-final.json` |
| Persisted policy controls | Pass | `.local/live-controls.json` |
| Route/detail review | 26 routes and 8 detail captures pass | `.local/rebuild-final/review.json`, `detail-review.json` |
| Visual, docs and container checks | Pass | `.local/visual-final.json`, `.local/rebuild-docs-final.log`, `.local/rebuild-container-final.log` |

## Remaining boundaries

- Local release follow-up: keep the final container/build artifacts and route captures
  reproducible after source integration.
- Separate qualification: the 100-execution/50-browser load corpus and cgroup scale work
  remain backlog items; campaign scheduling, private Gitea, finding Gitea, OIDC and repair
  runtime scenarios require disposable environments.
- External qualification: provider/model accounts, subscription entitlements, hosted
  topology, identity and isolation certification remain outside this local review.
- Product boundary: named policy presets and portfolio-wide automatic simulation are not
  exposed; simulation currently uses manual inputs for the selected repository as documented
  in the policy guide.

## Historical findings

The earlier review found unchanged shell hierarchy, insufficient runner/connection work surfaces, raw identifiers and JSON-heavy policy simulation. Those findings remain historical context; current source and fresh checks determine closure.
