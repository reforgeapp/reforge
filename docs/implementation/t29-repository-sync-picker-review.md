# T29 repository Sync picker review

Date: 2026-09-23

Status: local GUI fix complete; connected isolated-provider certification remains unrun.

The picker used a standalone `forge-connections` query key while Connections refresh invalidated the shared `connections` key. Its cached result could therefore stay disconnected from newly created or refreshed forge rows. The picker also queried before it was opened, so it had no explicit freshness boundary tied to the user's sync action.

The Sync dialog now fetches only when opened. It uses the shared `['org', orgID, 'connections']` cache prefix and zero stale time, so Connections mutations invalidate the same tenant-scoped query and each dialog opening refreshes the list. While fetching, it hides cached options and disables preview. Errors expose Retry and do not permit a preview with stale rows. Preview also requires the selected connection to remain in the current healthy list; a refresh that removes it clears the selection. `inventoryAPI.connections` retains its existing forge-kind filter, cursor pagination, repeat-cursor guard and bounded page count.

Validation:

- `npm --prefix web run build` passed; existing Vite large-chunk advisory remains.
- Focused `tests/repositories.spec.ts` passed 4/4. New browser regression begins with an older connection, then presents a new healthy Gitea connection after 100 earlier rows, verifies it can be selected, removes it during a live refetch and confirms preview is disabled, then closes/reopens the picker and reloads the page to verify the new row remains available through pagination.
- Attempted read-only browser confirmation against local `:8080` without route interception. Fixture-auth tenant had no healthy forge connections, so no real-row comparison could be made. Isolated acceptance endpoints `:8091` and `:8096` were unavailable (`HTTP 000`). No provider or customer repository was contacted or changed.

Remaining certification: rerun the connected reviewer journey against its isolated Go/PostgreSQL and disposable Gitea environment. Confirm the newly created healthy connection appears in the Sync picker after connection refresh, full reload and a fresh page, then continue only within the test's disposable repository and verify its cleanup.
