# T29 Audit page rebuild

Audit now uses a compact filter row, a readable event list, and a URL-backed detail panel. The list formats action names, shows the signed-in user’s name or a shortened account label, and omits event identifiers. Selecting an event shows full identifiers and event data; selection is URL-backed; X/Escape close the panel and restore row focus.

Repository, action, actor and time filters remain server-side. Search filters loaded rows in the browser. Export downloads the selected server-filtered page as NDJSON.

## Local verification

- `npm run build` — passed.
- Audit-focused Playwright coverage — 6 passed. Covers concise and advanced filters, URL-backed details, keyboard activation, runner entity labels, identifier placement, A-to-B selection with close-focus restoration, Escape/focus restoration and 390px overflow.
- Local PostgreSQL-backed development API — loaded 100 events; opened a real event, confirmed URL and full ID in details, closed panel, restored row focus; zero page errors.
- Screenshots were refreshed after the final changes: `.local/t29-audit-rebuild/audit-{light,dark}-{1440,390}.{png,jpg}` and `audit-dark-detail-1440.{png,jpg}`. Full-page PNGs preserve all loaded rows; viewport JPEGs are under 100 KB for review. Metadata: `.local/t29-audit-rebuild/metadata.json`.

This verifies local fixture-auth and development data only. It does not certify hosted identity, external tenants, or live audit-provider behavior.
