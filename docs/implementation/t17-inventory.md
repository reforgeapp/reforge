# T17 inventory GUI

Repositories page now uses persisted inventory routes and generated API types.

- Forge connection picker starts a preview scan, polls job state, exposes cancel/retry/error reasons, pages candidate preview and submits selected candidates with team assignment and `If-Match`.
- Repository list sends server filters for literal search, provider, team and status. URL stores filters and selected repository; local storage stores named filter views only.
- Detail fetches repository and native change cache, labels baseline and snapshot freshness, and links native records.
- Webhook API wrappers support inspect, issue/rotate and revoke. Connection pane wiring remains with the parent page.
- Development or missing backend states remain explicit; no synthetic repository records are used.

Integrated through SectionPage, scoped session/org cache and generated API types. Named views bounded to25, user+organisation-scoped and URL navigation preserves filters. Root browser checks:3pass; actual selected import/detail is covered by connections.spec.ts disposable private Gitea lifecycle. Baseline remains not yet recorded until a real T19 run.
