# T17 inventory GUI

Repositories page now uses persisted inventory routes and generated API types.

- Forge connection picker starts a preview scan, polls job state, exposes cancel/retry/error reasons, pages candidate preview and submits selected candidates with team assignment and `If-Match`.
- Repository list sends server filters for literal search, provider, team and status. URL stores filters and selected repository; local storage stores named filter views only.
- Detail fetches repository and native change cache, labels baseline and snapshot freshness, and links native records.
- Webhook API wrappers support inspect, issue/rotate and revoke. Connection pane wiring remains with the parent page.
- Repository detail includes server maintenance configuration for verified native bot actor IDs and requested merge authority, with exact-version CSRF/`If-Match` updates. Discovery status polls queued/running scans and exposes an actionable start/retry control.
- Development or missing backend states remain explicit; no synthetic repository records are used.

Integrated through SectionPage, scoped session/org cache and generated API types. Named views bounded to25, user+organisation-scoped and URL navigation preserves filters. Private Gitea browser lifecycle now completes private import, observe-mode maintenance configuration, discovery scan, persisted discovery reopen, credential rotation/retest, webhook lifecycle, and final revoke against disposable local Gitea.
