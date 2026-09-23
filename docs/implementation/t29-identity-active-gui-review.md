# T29 active Identity GUI

Date: 2026-09-23

Status: active state and organisation sign-in action implemented; connected local acceptance passed against runtime commit `08669b4`.

An active configuration now reads “Login active”. Activate is only shown when the server says the saved version is eligible; it disappears after activation. The active state offers `/auth/login?org_id=<UUID>` in a new tab. Disable remains available. Metadata probing remains a separate action.

Validation:

- `npm --prefix web run build` passed; Vite retains its existing large-chunk advisory.
- Focused `tests/organisation.spec.ts` passed 12/12 against local Vite with fixture responses. Coverage includes activate, active label, no unavailable/draft state, sign-in link keyboard activation, reload, confirmed disable, dark/light screenshots, mobile width and closed drawer.
- Captures are ignored under `.local/t29-identity-gui/`; they use fixture responses and contain no real secret.
- Connected browser acceptance passed against Go server `127.0.0.1:8092` at `08669b4`, restricted-role PostgreSQL `127.0.0.1:55434/reforge_identity`, migrations 001–035, and a disposable RSA-signed OIDC issuer on `127.0.0.1:8093`. Real API results: save 200, metadata probe 200, activation 204, confirmed disable persisted. Reload retained `Login active` and the org-specific sign-in URL. Keyboard Enter followed the link through authorization and callback to the organisation overview; the resulting session contained exactly the pre-enrolled member and target org. The runtime role reports `rolsuper=false, rolbypassrls=false`; isolated DB check confirmed the submitted secret was absent from the stored envelope plaintext. Final config was disabled at version 6.
- Live dark/light 1440px and 390px captures are in ignored `.local/t29-identity-active-gui/artifacts/identity-active-*.png`; mobile acceptance checked closed drawer and no horizontal overflow. The local IdP used no external or paid API. Evidence summary: `.local/t29-identity-active-gui/artifacts/live-acceptance.json`.
