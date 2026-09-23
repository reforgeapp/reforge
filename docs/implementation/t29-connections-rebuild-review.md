# T29 Connections GUI rebuild review

Status: locally reviewed and verified. Live provider certification remains outstanding.

## Result

Rebuilt the Connections inventory with aligned category, search and state controls, list-scoped refresh and a primary Add action. Details open in a split panel with a top-right close control and Escape support. Mobile list rows use compact cards. Removed the credentials/server-probe explanation.

Create, test, rotate, credential versions, private runner routes, model/agent qualification, custom command profiles and forge webhook actions remain connected to the backend. Connection revocation now requires a named confirmation dialog before its version-checked DELETE. Model and agent rows use separate server-filtered pagination streams, so a forge-heavy first page cannot hide later models or agents. Counts say “shown” while another page remains. If local filters hide every loaded row but another page exists, the empty state says results are limited to loaded pages and retains Load more.

## Checks

- `npm run typecheck` and `npm run build` passed. Build has the existing large-chunk warning.
- Five focused browser checks passed: stale rows remain visible after refresh failure, models remain visible when the agent stream fails, explicit revoke confirmation preserves version/CSRF, loaded-filter pagination, and detail close/Escape focus restoration. Existing Connections browser checks previously covered route persistence, model/agent pagination, and the narrow keyboard dialog.
- Against a separate disposable database/server, created and opened a Gitea connection through the GUI, then exercised the Test capability API against a reserved `.invalid` endpoint. It returned the expected degraded state. The server/database were removed afterward.
- Fresh viewport-sized browser captures cover dark Add and loaded Detail at 1440×900 and 390×844. Detail capture waited for the named heading, matched the detail response ID to the URL, rejected the loading state, and asserted no page errors. All captures asserted the DOM theme is dark; mobile captures additionally assert the navigation drawer is offscreen, `aria-expanded=false`, document width stays within 390px, and title/status precede the action row. Older dark captures were not trusted.
- Screenshots: `.local/t29-connections-rebuild/{detail,add}-dark-{1440,390}.jpg`.

## Limits

No real provider endpoint or credential was used. This verifies local API/UI create and probe handling, not successful live forge, model, agent or delivery capability. Secrets were not shown or captured.
