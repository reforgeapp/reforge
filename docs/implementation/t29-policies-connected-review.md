# Connected admin acceptance after Policies rebuild

Date: 2026-09-23

The connected admin Playwright flow now follows the rebuilt Policies page: save reason and immutable version are accessed in Review; malformed raw JSON is entered under Scope → Advanced policy JSON and rejected from Review. It verifies save is unavailable outside Review and that Acceptance policy history is absent in Development. Runner validation, creation, draining, navigation/reload persistence and tenant isolation remain covered.

The test ran without API/session route interception against a fresh loopback-only PostgreSQL 18.6 cluster and Go server at `127.0.0.1:8091`. Database `reforge_admin_acceptance` was isolated from the shared Development database. Fixture authentication created the Development owner; the disposable database had a separately seeded Acceptance organisation, owner membership, Gitea fixture connection, and the repository expected by the test. The migration role had `BYPASSRLS`; the runtime role was neither superuser nor `BYPASSRLS`. No provider endpoint was contacted.

Validation:

- Current frontend TypeScript/Vite build passed into `.local/t29-policies-connected/web-dist`; current Go server build passed.
- `connected-admin-acceptance.spec.ts` — 2 passed. The backend returned 400 for the overlength runner name, accepted pool creation and update, and accepted both immutable policy version saves. Browser assertions verified malformed JSON feedback, persistence after navigation/reload, and tenant isolation.
- Direct readback from the isolated database showed one Acceptance runner pool in `draining` state, two Acceptance organisation policy versions, and no Development policy versions or runner pools.
- Playwright screenshots for Policies at 390×844 and 1440×900 in light and dark themes are under `.local/t29-policies-connected/artifacts/`; all four were visually inspected. Overview mobile screenshots and `admin-backend-requests.json` are in the same directory.
- Served JS SHA-256: `498f1ea29b5833f68956b65c2cef11c32a719ea214d6626c237738211266b59c`; CSS SHA-256: `99728aba59d527aea69ea3d0fbad3c16ea1a2c6fe9d2eaf9438b22898ac485a4`; Go server SHA-256: `74af9cd6e80d8973f4e4998992fa57ecb1ee93625bf41ee3f6862cb28df84844`.

The server and dedicated PostgreSQL cluster were stopped after the run. The database files and build/screenshots remain in ignored `.local/t29-policies-connected/` for review. This is local fixture-auth evidence only; it does not certify external provider behavior.
