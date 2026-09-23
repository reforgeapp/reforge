# T29v invitation slice — Luna final report

Date: 2026-09-23. No commits.

## Result

Invitation UI/API and contract complete. Strict-CSP connected acceptance passed against disposable PostgreSQL and a local signed RS256 issuer. `cspBypass=false`; no API interception.

Owner flow created and copied an invitation, displayed accepted state after reload, and revoked another pending invite. Invitee redemption returned 200 JSON authorization URL, completed signed callback, and created exactly one verified identity, reviewer membership, and organisation session. Replay and revoked-link attempts returned 401 with membership count unchanged. Token fragment was removed before request; token absent from HTTP request URLs, server logs, browser storage, and rendered error states. Keyboard submission, Escape dismissal, 390px light/dark owner/invite layouts passed without horizontal overflow. Strict run evidence: `.local/opus-resume/identity/artifacts/connected-acceptance.json`; captures alongside it.

## Source

- Backend: `internal/httpapi/invitations.go`, `internal/httpapi/invitations_test.go`. Successful redemption returns 200 JSON `{authorization_url}`; same-origin form body, origin/fetch metadata, query/body bounds, duplicate token and no-store/no-referrer protections remain enforced.
- GUI/API: `web/src/app/OrganisationPage.tsx`, `web/src/organisation-api.ts`, `web/src/app/InviteLandingPage.tsx`, `web/src/main.tsx`, `web/src/styles/organisation.css`, `web/src/styles/invite.css`, `web/tests/organisation.spec.ts`, `web/tests/invitations.spec.ts`.
- Contract: `docs/implementation/t29-identity-invitations-contract.md`.
- `web/src/app/AppShell.tsx` invite bypass line is preserved; browser worker now owns that file and is handling the separate resize navigation overlay finding.

Invitation GUI exposes cached list refresh/pagination errors with retry, keeps rows visible during refresh failures, and places clipboard/revoke failures in their dialogs. Token remains memory-only.

## Verification

- `cd web && npx tsc --noEmit` — pass.
- `cd web && REFORGE_BASE_URL=http://127.0.0.1:5173 npx playwright test tests/invitations.spec.ts tests/organisation.spec.ts --output /tmp/reforge-opus-resume/identity-tests` — 20/20 pass.
- Retained `.local/opus-resume/integration/identity-http-fresh-race.log` — fresh restricted-role OIDC/auth and HTTP race suite passed. Coordinator additionally ran final current-handler invitation HTTP race tests; `.local/opus-resume/integration/invitation-http-final-race.log` passed.
- `.local/opus-resume/identity/web-build.log` — production build passed (existing chunk-size advisory).

## Limits

Local signed issuer is contract evidence, not hosted IdP certification. Browser worker reproduced `navOverlayAfterResize: true` as an intermediate CSS transition measurement; settled geometry passes. Fresh 390px loads passed. Other in-flight shared work remains uncommitted and untouched here.

Confidence: 96%.
