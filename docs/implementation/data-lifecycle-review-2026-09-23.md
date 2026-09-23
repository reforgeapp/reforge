# Data lifecycle review — 2026-09-23

T29q locally complete. Baseline `64d783b`. Three Luna workers owned disjoint admin lists,
work lists and Organisation changes. Astra reproduced failures, reviewed every change and
ran integrated verification. No backend contract, tenant authority or persisted demo data changed.

## Findings and repair

1. Connections and Runners cleared local row copies on refresh. Identical server responses
   retained their React Query identity, so effects did not repopulate those copies. Repeated
   Connections tabs and repository saved views could also empty otherwise populated lists.
   Findings lost loaded pages when returning to a cached filter. Five main lists now derive
   deduplicated rows from query-owned pagination; old resource invalidation prefixes remain.
2. Repositories expected a finite `teams` response under the same key Organisation used for
   infinite pages. SPA navigation crashed; cold reloads hid the collision. Infinite lists now
   use distinct keys. Team writes invalidate all affected team queries.
3. List loading replaced whole pages, unmounting filter inputs. Live runner typing `demo`
   produced `d` and lost focus. Filter controls now remain mounted while the list displays
   loading/error states. No cross-scope placeholder data is used.

Refresh retains rows while fetching, then accepts updated, removed or empty server results.
Pagination is disabled during refresh so loading another page cannot cancel reconciliation.
Detail/runner/webhook descendants retain broad invalidation; existing signals cancel old tenant
requests. Connection filters also include the backend's degraded state.

Other main routes already derive lists directly from finite/infinite query data. Their cold,
warm and reload states were reviewed alongside the repaired routes. No additional incompatible
active cache shape was found. Existing external qualification limits remain unchanged.

## Evidence

Artifacts: `.local/data-lifecycle-2026-09-23/`.

| Artifact/check | Result |
| --- | --- |
| `before.json`, `before.log` | Development 33 → 0 displayed connections after Refresh despite 33 API records; Demo Operations 1 → 0. Same/cached-tab returns also reproduced |
| `before/navigation.json` | 86 states; both organisations' Repository → Organisation transition showed the router error boundary; Connections/Runners refresh lost rows |
| `regression-baseline-final.json` | Corrected 12-case baseline: six failures, six passes. Failures cover unchanged admin refreshes, both directions of team-cache navigation, cached finding pages and saved-view reload |
| `typing-before.json` | Both sequential-typing regressions failed before stable-toolbar repair |
| `browser-final.json` | **152 passed, zero failed, six skipped**; includes 15 new lifecycle regressions, existing workflows, accessibility, narrow layouts and optional visual captures |
| `after.json`, `after.log` | Development retains 33 and Demo Operations retains one forge connection through Refresh, same-tab click, cached-tab return and reload; Models tab finishes loading normally |
| `after/navigation.json` | **86 states passed** across 13 routes and both organisations; cold/warm/reload row counts agree, refresh preserves rows, no missing application shell or uncaught page errors |
| `typing-live.json` | Actual backend: `demo` retained with focus at 1440px and 390px; corresponding screenshots retained |
| `build-final.log` | TypeScript and production frontend build passed; existing large-chunk advisory remains |
| `container-build.log` and container smoke | Build passed; uid 10001, executable/migrations present, no Docker socket; packaged assets match served bundle |

New regressions cover identical held responses, cached tabs, replacement/removal after multi-page
refresh, retry, late old-organisation requests, finding page overlap/deduplication and genuine
empty filters, saved views, team mutation invalidation, cached run filters and continuous typing.
Fixture tests model these states; connected checks use the existing local PostgreSQL-backed app.

Initial test drafts included a wrong run-ID selector and hard navigations that reset the cache;
review corrected them before the authoritative baseline above. The first Models audit sampled
a loading state; the retained final audit waits for list loading to finish.

Six skipped opt-ins: campaign persistence, private Gitea connection lifecycle, imported finding
triage, policy/budget/audit persistence, non-fixture OIDC and isolated full repair. Earlier evidence
for these is unchanged. No external API certification or G5 completion claimed.

## Delivered build

Demo remains `http://127.0.0.1:8080`; reload once to load the repaired bundle. Existing app,
PostgreSQL and docs services retained; no extra demo stack created and no reseeding performed.

Assets: `index-DagGakBn.js`, `index-Dsnuv0cZ.css`.
Control image `reforge:gui-review`:
`sha256:3dfb4c3fd54b266a5bf5c9dda445cff25ca3ad09ceb4ca8a747a54f404ee56f0`.

No T29q blocker remains. Broader release/certification tickets remain open as recorded in progress.
Owner changes to `copilot-handoff-2026-09-22.md` preserved and excluded from this task.
