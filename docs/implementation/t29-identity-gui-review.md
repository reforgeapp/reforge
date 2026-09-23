# T29 organisation Identity GUI

Date: 2026-09-23

Status: owner-only configuration GUI implemented; connected local acceptance passed against the committed configuration/probe backend slice.

Identity now uses the organisation OIDC API for read, versioned draft save, metadata probe and confirmed disable. The client secret is write-only: form clears it after save and never fills it from responses. Existing secrets are preserved when the field is left blank. Mutations send `If-Match` and CSRF. The status row can refresh shared owner state without discarding unsaved fields. Admins no longer see owner-only Members or Identity tabs.

Probe status says metadata was verified. Login activation stays disabled while `activation_available` is false; server blocker is available through a keyboard-focusable `?` tooltip. No sign-in activation is claimed.

Validation:

- `npm --prefix web run build` passed; Vite retains its existing large-chunk advisory.
- Focused `tests/organisation.spec.ts` passed 12/12. Covered session/role boundaries, draft creation and reload, write-only secret behavior, If-Match/CSRF, successful probes, failed probes with failed follow-up reads, same-version status refresh preserving edits, disabled activation, confirmed disable, read retry, keyboard tab navigation and 390px layout.
- Browser captures are in ignored `.local/t29-identity-gui/`: light/dark at 1440px and 390px. Captures use test-only responses; no real secret is present. Mobile checks assert drawer closed, sidebar offscreen and no horizontal overflow.
- Connected browser acceptance passed against isolated Go server `127.0.0.1:8092` built from `d2c6175` and PostgreSQL `127.0.0.1:55434/reforge_identity`, with migration 034 applied. Browser used fixture-auth owner and a disposable loopback metadata issuer; no provider or paid API was used. Real API results: save 200, failed metadata 422, immediate retry 429 with `Retry-After`, recovery probe 200, confirmed disable persisted, and admin read denied 403. Reload retained issuer/client ID and did not return the secret. Direct isolated-DB check found the stored envelope does not contain the submitted secret; runtime role reports `rolsuper=false, rolbypassrls=false`. The test switched the disposable fixture membership to admin, verified Identity was hidden and GET denied, then restored owner.
- Light/dark 1440/390 captures remain test-response captures; no real secret is present in any capture.
- This connected run used the committed `d2c6175` backend binary/schema at migration 034. It does not certify the in-progress runtime follow-on files/migration 035 or hosted login activation. The d2c6175 response reported activation unavailable; final runtime activation behavior remains pending the follow-on slice.

Remaining: rerun connected acceptance against the final runtime follow-on schema/API after it is committed, and confirm activation stays disabled with the final `activation_available=false` response. Do not claim hosted identity login; org-aware login callback is not implemented.
