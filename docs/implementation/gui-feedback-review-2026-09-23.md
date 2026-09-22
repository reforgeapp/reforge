# GUI feedback review — 2026-09-23

T29g–o locally complete. Owner resumed work after Deepseek commits `80fce9c`, `862c0c4`
and `b7f2a11`. Astra reviewed; Luna implemented. No external provider mutations or paid APIs.
Human feedback, provider certification and G5 release qualification remain separate.

## Changes

| Ticket | Result |
| --- | --- |
| T29g | Backend-connected stored-evidence impact preview; scoped sequential simulations, bounded pagination, cancellation, actual effective-policy bindings, gate deltas and collapsed backend blockers. Missing evidence cannot become an allowed result. |
| T29h/i | Warm-cache repository navigation and nullable Organisation payload fixes verified. |
| T29j | Organisation dropdown/More modal, descendant clicks, arrows, Tab/Shift+Tab, Escape and focus restoration repaired. |
| T29k | Deepseek Campaign/GitOps layout changes retained; Campaign State filter narrowed to 240px with adjacent action. |
| T29l | Attached top-right X, Escape, deep-link fallback and row focus restoration. Shared detail panels preserve tab/filter focus during async loads and remounts. |
| T29m | Neutral charcoal dark surfaces, softened accents/status colours; system preference, persistence and unavailable-storage behavior verified. |
| T29o | Responsive Policies editor/history workspace, compact presets, shared reason/save controls and human-readable caps; advanced JSON/manual simulation retained. |
| T29n | Integrated browser, accessibility, responsive, real-control and packaging checks recorded below. |

Policy authority, RBAC/CSRF, immutable versions, activation hashes/CAS, zero caps and
native approvals remain server-controlled. Preview never activates policy or calls providers.
Empty repository previews send blank evidence bindings; backend denies remain denies and
unsupported allows become unknown. Unsaved edits and access changes invalidate previews.

Reviewed implementation commits: `efa4a1a`, `8e28ef2`, `b66cbad`, `afed50a`, `5be8fed`,
`f6b2c30`, `4e2555b`, `e5b53e7`. Delivered application source ends at `e5b53e7`;
following documentation commits do not change the served application.

## Final evidence

Artifacts: `.local/resume-2026-09-23/`.

| Check | Actual result |
| --- | --- |
| `browser-accepted.json` | 133 passed, 0 failed, 9 opt-in skipped; final application behavior before the last Campaign-only CSS adjustment |
| `campaigns-delivered.json` | 5 passed after that scoped CSS adjustment |
| `affected-final.json` | 17 passed; revalidation keyboard controls and all ten policy-impact cases |
| `delivered/review.json` | 52 route/theme/viewport checks; zero browser errors, axe violations or document overflow; loaded Findings focus restoration passed at all four combinations |
| `live-controls.json` | 1 passed: real database policy activation, persisted zero budget, narrow controls and audit NDJSON export |
| `navigation.json` | 20/20 warm-cache, tenant-switch, Organisation and docs checks |
| `check.log` | `make check` passed: Go vet, frontend build and generated API/SQL drift checks |
| `build-final.log` | Delivered production frontend build passed |
| `docs-build.log` | Strict MkDocs container build passed; current policy guide, version archive and 103-document search index served |
| `control-build-delivered.log` | Delivered control image built; executable server/migrator/evidence binaries, migrations and matching frontend assets verified under uid 10001; no Docker socket |

Browser suite skips: scheduled campaign API lifecycle; runner/private-connection lifecycle;
live finding triage; live policy/budget/audit controls; non-fixture OIDC; real repair/native
publication; three visual-capture scenarios. The live controls test ran separately and passed.
Dedicated delivered captures cover the GUI matrix; skips do not establish external certification.

Final assets: `index-DuIIVppu.js`, `index-BkjLdBaM.css`. Chromium `148.0.7778.96`.
Matrix: 13 routes × light/dark × 1440×900/390×844; extra loaded Findings and policy-impact
screens included. Real Demo Operations preview returned six repository decisions, including
two repositories without stored work; missing evidence and backend denials remain visible.

Images:

- `reforge:gui-review`: `sha256:2feb7a79e94b408d9d8b06a6784d7df61f1a1a0c5b4a0be64b7631837eaeeaa2`.
- `reforge-docs:review`: `sha256:f78f6c4ce13ee1d9382b3d26ea150741467a44ff2e17e7c677264d2f428af299`.

Go/API surfaces did not change. Historical full Go integration, clean-install, race and
restore results were not rerun or recertified by this frontend review.

## Review failures preserved

- `before/`: 16 baseline captures; served `index-D1yu7M0w.js`, source `b7f2a11`.
- `focused.json`: 50 passed, 1 failed, 1 skipped; identified Findings focus failure.
- `layout.json`: 26 passed, 2 failed, 1 skipped; Findings focus and fixed organisation-count assumption.
- `browser-final.json`: 131 passed, 1 failed, 9 skipped; Review-tab focus stolen on remount.
- `browser-verified.json`: 131 passed, 2 failed, 9 skipped; async focus theft plus a zero-repository fixture selecting a nonexistent option.
- Review caught a false-positive empty-repository assertion matching another repository's
  unknown result. Final test checks the specific request, blank binding and repository row.
- Earlier `final/`, `verified/` and `accepted/` captures are intermediate build identities.
  Use `delivered/` for current screenshots. Final loaded captures replace loading-state evidence.

## Local demo and launch

Open `http://127.0.0.1:8080`, sign in, then choose **Demo Operations**. Direct route:
`http://127.0.0.1:8080/org/00000000-0000-4000-8000-0000000000de/overview`.
Docs: `http://127.0.0.1:8082/docs/`.

One review stack remains: app PID `3299460`; PostgreSQL PID `2952266` on
`127.0.0.1:55432`; docs container `reforge-review-docs` on port 8082.
Recheck command lines before stopping any recorded PID. App metadata includes
`REFORGE_DOCS_URL=http://127.0.0.1:8082/docs/`. GUI serves `web/dist` directly.
[Startup/restart commands](gui-rebuild-review-2026-09-22.md) remain applicable; old PIDs
there are historical. Source local environment without printing its credential values.

Development authentication and `.invalid` integrations are intentional. Empty execution
history reflects persisted demo state; fixtures do not certify production integrations.

## Remaining release actions

1. T08/T09/T12–T16/T22/T23: provide authorised disposable GitHub/GitLab/Gitea and model/
   subscription accounts, permitted runtime/topology/entitlement evidence and explicit paid
   API budget. Run required native approval, protection, model and deployment contracts.
2. T26: provide customer OIDC issuer and hosted infrastructure for installation/isolation
   certification. Kubernetes mutations remain GitOps-only.
3. T27: run remaining hostile corpus on a host with working cgroup delegation. Hosted
   untrusted execution stays disabled until its isolation gate passes.
4. T28: prove 100 fully executing runs and 50 real browser sessions with adequate runner/
   browser resources. Existing claim/HTTP-session tests do not satisfy those targets.
5. T28: finish outstanding dependency/licence inventory review and owner release sign-off.
   Publication or external deployment needs separate authorisation. G5 remains open.

The goal API exposed no resume operation while its old status was blocked; explicit owner
resumption authorised this completed GUI work. Owner edits to
`copilot-handoff-2026-09-22.md` remain untouched and uncommitted.
