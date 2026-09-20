# T04 shell evidence

The React shell is scoped by organisation in deep links (`/org/:orgID/:section`) and uses TanStack Query keys beginning with `org`, so a switch cancels and removes cached tenant queries before navigation. Search is kept in the URL as `q`, and repository loading uses the frozen `GET /api/v1/orgs/:orgID/repositories` endpoint with cursor and limit support in the client shape.

The UI has the slate navigation/light content/teal action treatment from the prototype, responsive navigation, a skip link, visible focus, a native modal dialog with focus on open and restore on close, status/gate badges, and explicit loading, error, stale, empty and blocked component states. It renders no prototype business records. The server's `meta.development` and `meta.fixture_auth` values are shown as a visible banner. Authentication remains session based: 401 renders the OIDC sign-in state and the login action navigates to `/auth/login`; logout sends the session CSRF token in `X-CSRF-Token`.

Checks completed locally:

- `npm run typecheck`
- `npm run build`
- Playwright checks are configured for `npx playwright test` against the running local server. They were not completed in this checkout because PostgreSQL was unavailable to the T02 identity binary; the restricted browser runner also requires `PLAYWRIGHT_CHROMIUM_PATH` when its bundled revision is absent.

The repository, finding, run, change, deployment and administration routes are shell placeholders for their owning tickets. They intentionally contain no synthetic records or mutation claims. Full acceptance requires T02 identity, later persisted portfolio endpoints, event transport and the integrated browser matrix.
