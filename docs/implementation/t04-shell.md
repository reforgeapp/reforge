# T04 shell evidence

The React shell is scoped by organisation in deep links (`/org/:orgID/:section`) and uses TanStack Query keys beginning with `org`, so a switch cancels and removes cached tenant queries before navigation. Search is kept in the URL as `q`, and repository loading uses the frozen `GET /api/v1/orgs/:orgID/repositories` endpoint with cursor and limit support in the client shape.

The UI has the slate navigation/light content/teal action treatment from the prototype, responsive navigation, a skip link, visible focus, a native modal dialog with focus on open and restore on close, status/gate badges, and explicit loading, error, stale, empty and blocked component states. It renders no prototype business records. The server's `meta.development` and `meta.fixture_auth` values are shown as a visible banner. Authentication remains session based: 401 renders the OIDC sign-in state and the login action navigates to `/auth/login`; logout sends the session CSRF token in `X-CSRF-Token`.

Checks completed locally:

- `npm run typecheck`
- `npm run build`
- Session queries poll active tabs every 30 seconds and tenant queries are cancelled/removed on 401 or user identity changes. `PLAYWRIGHT_CHROMIUM_PATH=/home/mnorris/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome npx playwright test` passed against the live T02 development server: 8 tests passed in 2.7 seconds. The suite covers sign-in and the explicit fixture banner, protected and invalid organisation deep links, deep links and URL search with back restoration, keyboard focus, narrow viewport navigation, organisation dialog focus/escape handling, same-page session-revocation cache purge, and axe accessibility checks. The session-revocation check uses Playwright route instrumentation with a labelled sensitive repository fixture as an isolated shell check; it does not certify backend revocation. The cached Chromium requires escalated local service access in this runner.

The repository, finding, run, change, deployment and administration routes are shell placeholders for their owning tickets. They intentionally contain no synthetic records or mutation claims. Full acceptance requires T02 identity, later persisted portfolio endpoints, event transport and the integrated browser matrix.

Coordinator review also gates rendering across identity changes until scoped cache clearing completes; no previous-user content is painted during that transition.
