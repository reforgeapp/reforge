# T29 Organisation GUI rebuild review

Teams and Members now use list/detail flows with explicit create, edit, save, and remove actions. Writes use the existing version-checked API, and membership edits retain staged team and repository scopes when All repositories is selected. Empty, loading, error, retry, and pagination states are covered. Session loading/error now resolve before role checks, and the empty Teams state exposes one Create action. Identity remains unavailable until its backend contract exists; the page shows one inline pending status and does not present fake settings. This tab is not accepted as complete.

## Verification

- `npm run typecheck` and `npm run build` passed. Focused Playwright tests passed 9/9, including pending-owner session, retryable session error, and the single empty-state Create action.
- Focused Playwright tests passed 6/6: `organisation.spec.ts` and `organisation-scope-status.spec.ts`.
- Browser checks used an isolated PostgreSQL database and fixture-auth server on loopback. Team scope changes and membership role/scope changes returned HTTP 200 and remained after reload. The disposable database and server were stopped after the run.
- Desktop captures are in `.local/t29-organisation-rebuild/captures/` (ignored local artifacts): Teams empty/detail and Members detail, with light/dark variants. Narrow-screen Teams and Members light/dark captures are included. A separate intercepted-fixture browser pass asserted the drawer was fully offscreen and document width stayed within 390px; earlier transition captures were removed.

## Remaining backend dependencies

- Membership responses expose only user IDs, role, and scope. The UI masks IDs in the member list; useful names and email addresses require a tenant-scoped member-profile read API before membership management is production-usable.
- Identity has no application API for reading or changing sign-in settings. Enable configuration only after the backend contract and authorization checks are implemented.

## Limits

The isolated fixture verified the existing team and membership endpoints, but did not certify cross-tenant authorization or production identity configuration. Vite also reports the existing large JavaScript chunk warning.
