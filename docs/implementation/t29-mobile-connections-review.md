# T29 mobile Connections

Status: local responsive fix complete. The narrow table now shows Name, Provider, State and Open in the viewport; credential version stays available in the desktop table. Search, state filters, provider tabs and backend behavior are unchanged.

Before and after captures use the same disposable PostgreSQL database, Development tenant and persisted Gitea connection at 390×844 and 1440×900. Before images show the narrow table clipped after State. After images show all four useful columns. The desktop table retains all five columns. Mobile provider cells wrap long identifiers such as `custom_command` and `claude_code` inside their column.

| Viewport | Before | After |
| --- | --- | --- |
| 390×844 | [PNG](../../.local/t29-mobile-connections/before-connections-390.png) | [PNG](../../.local/t29-mobile-connections/connections-after-390.png) |
| 1440×900 | [PNG](../../.local/t29-mobile-connections/before-connections-1440.png) | [PNG](../../.local/t29-mobile-connections/connections-after-1440.png) |

Dark mobile capture: [390×844 PNG](../../.local/t29-mobile-connections/connections-after-dark-390.png).

At 390px: viewport/document width 390px; table/wrapper/content width 366px. At 1440px: viewport/document width 1440px; table width 1184px. No page or table horizontal overflow after the fix.

Checks: `npm --prefix web run build`; `REFORGE_CONNECTIONS_MOBILE_URL=http://127.0.0.1:8096 npm --prefix web exec playwright test tests/connections-mobile.spec.ts --reporter=line` (1 passed). Browser used actual Go and PostgreSQL APIs, created or reused persisted fixture connection, reloaded list, probed `custom_command` and `claude_code` text bounds inside the mobile provider cell, opened and closed detail with keyboard, and verified focus restoration. API errors: 0.

Evidence and measured widths: `.local/t29-mobile-connections/widths.json`. Database and app used loopback-only disposable services with explicit fixture auth; no shared demo service or real provider was contacted.
