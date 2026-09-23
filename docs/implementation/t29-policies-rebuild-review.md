# T29 Policies page rebuild review

Date: 2026-09-23

Policies now separates scope context, policy editing, and review. The scope selectors sit beside a compact effective-state strip; inherited evidence and rules JSON stay collapsed. Field tabs contain policy controls only, including schema under Advanced policy JSON. Save reason and immutable-version save live in Review with impact preview and candidate simulation. Version history remains beside the editor on desktop and collapses below it on narrow screens.

The editor preserves organisation, repository, and team scope, role checks, draft presets, raw JSON, immutable versions, impact simulation, rollout simulation, activation, CSRF, stale-data handling, and query errors. No API or database contract changed. Generic helper paragraphs were removed.

Validation:

- `npm run build` — passed (TypeScript and production Vite build).
- `npx playwright test tests/policy-editor.spec.ts tests/policy-impact.spec.ts --workers=1` — 21 passed. Covers scope and role boundaries, presets, save payloads, invalid JSON, impact pagination, stale/incomplete evidence, simulation requests, and responsive review flow.
- Browser viewport check passed at 390×844 with no document-wide horizontal overflow. Desktop and narrow-screen light/dark themes were captured and visually reviewed.
- Captures: `.local/t29-policies-rebuild/policies-1440-light.png`, `.local/t29-policies-rebuild/policies-1440-dark.png`, `.local/t29-policies-rebuild/policies-390-light.png`, `.local/t29-policies-rebuild/policies-390-dark.png`.

Playwright uses intercepted API fixtures. These checks do not certify live policy behavior against a production backend or external providers.
