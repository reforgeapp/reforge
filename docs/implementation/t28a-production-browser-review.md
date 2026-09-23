# T28a production browser review

Built final source `675b0fdc0bb315c44506dc417cfa1f686d116a4a` in clean detached worktree `/tmp/reforge-production-browser-675b0fd`. `npm run build` and `go build -trimpath ./cmd/server` passed. Go binary SHA-256: `fe052c4455eb8f53133aba18219fb3e57650de6b02523a0c1104d3bd5508a478`. Production web bundle SHA-256: `8eaa57f870af0e492688b373bb5abd19b19900995801a3313baf797aa646866c`; retained screenshot bundle hashes match byte-for-byte.

Final one-worker Playwright suite: **193 passed, 15 skipped, 0 failed** (208 total, 1.9 minutes). Log: `.local/opus-resume/production-browser/full-suite-final.log`; result: `.local/opus-resume/production-browser/playwright-results-final/.last-run.json` (`passed`, no failed tests). Trace mode was retain-on-failure; no failure traces were generated.

The 15 skips require external or explicit opt-in environments: `install-oidc.spec.ts` (non-development issuer); `repair-live.spec.ts` (Gitea, Ollama, runner, gVisor); `visual-states.spec.ts` (2) and `visual.spec.ts` (1) (`REFORGE_VISUAL=1`); `insights-live.spec.ts` and `campaigns-live.spec.ts` (disposable local DB opt-ins); `connected-admin-acceptance.spec.ts` (2, isolated backend 8091); `findings.spec.ts` and `connections.spec.ts` (imported repository / disposable Gitea); `connections-mobile.spec.ts` (isolated fixture app); `connected-acceptance.spec.ts` (2, isolated backend 8090); and `connected-reviewer.spec.ts` (isolated backend 8091 plus disposable Gitea). None was skipped due to failure.

Sixteen Chromium screenshots cover Policies populated with an unsaved draft, Connections inventory and controls, DB-backed Audit event detail, and Organisation Identity invitations at 1440×900 and 390×844 viewports in light/dark themes. See `.local/opus-resume/production-browser/captures/metadata.json`. Authentication and Audit events came from disposable PostgreSQL. Policy, connection inventory, OIDC settings, and invitation rows came from screenshot-only GET fixtures; capture issued no POST, PUT, or DELETE. Longer views were full-page captures. Narrow screenshots waited until sidebar geometry was off-canvas (`right <= 1px`) and no backdrop remained.

## Containerized demo runtime

Demo serves at `http://127.0.0.1:8084` in container `reforge-t28c-final-demo` (ID `2ced6e2d6b270f0407bad4c117e1f865e88287177ff7f0a28278b927664d0902`), image `reforge-t28c-final-control:675b0fd` (image ID `sha256:59819fbd3a751b824117728157934a1b75af1228901848d1c9794c5499f17da0`). Host networking lets app reach existing PostgreSQL at `127.0.0.1:55437`; app binds loopback `127.0.0.1:8084`, with no externally exposed host port.

From `/home/mnorris/repos/reforge`, run `sh .local/opus-resume/demo/start.sh` to start and await PostgreSQL, docs, then app readiness. Run `sh .local/opus-resume/demo/stop.sh` to stop app container only; DB and docs remain running. Docker daemon must be running. App, DB, and docs use `unless-stopped` restart policy. DB is existing `reforge-opus-integration-pg` (PostgreSQL 18.6, port 55437); docs are existing `reforge-t28c-current-docs-serve` (image `reforge-t28c-final-docs:675b0fd`, port 8082).

App runs as nonroot host UID/GID `1000:1000`, with read-only root filesystem, all capabilities dropped, `no-new-privileges`, 256 process limit, and private `/tmp` and `/app/var` tmpfs. Existing private artifact directory `.local/opus-resume/production-browser/artifacts-final-runtime` is bind-mounted at `/app/var/artifacts`; it remains UID 1000, mode 0700. Runtime env file `.local/opus-resume/demo/runtime.env` is mode 0600 and contains existing DB URL/encryption key plus fixture auth and loopback/docs settings; values are never printed.

After app-container restart, same PostgreSQL-backed Audit event remained accessible. Authenticated GUI, `/readyz`, fixture-auth metadata, normal CSP, and actual Policies Help link `http://127.0.0.1:8082/docs/policies/` (HTTP 200) were verified on container.

Host launcher `.local/opus-resume/production-browser/start-server.py` remains a manual fallback only. Obsolete host PID is archived at `.local/opus-resume/demo/host-server.pid.obsolete`; use Docker scripts above for normal lifecycle.
