# T19 browser repair authority

Run the harness only with a disposable local database and verified runtime image mapping:

~~~sh
set -a
source .local/development.env
set +a
make build
export REFORGE_BROWSER_RUNTIME_CONFIG=/private/reforge-runner/runtime-config.json
export PLAYWRIGHT_CHROMIUM_PATH=/path/to/chromium
export REFORGE_REPAIR_IMAGES='{"go":"sha256:<64 lowercase hex characters>"}'
python3 scripts/test-repair-browser.py
~~~

For an explicit local demo that remains reviewable after the browser exits, add `REFORGE_KEEP_DEMO=1`. This retains only `.local/repair-demo/`, its disposable PostgreSQL database, the localhost controller on `127.0.0.1:8081`, and `.local/demo-repair.json` (mode `0600`). The manifest records exact organisation, run, and native change URLs after a successful repair and contains no credentials. Default runs still stop the controller, drop the database, and remove temporary artifacts. Stop a retained demo with:

~~~sh
set -a; source .local/development.env; set +a
python3 scripts/test-repair-browser.py --cleanup-demo
~~~

The cleanup mode validates recorded process start times, executable paths, localhost database naming, fixture repository naming, and owned credential paths before stopping or deleting anything.

The script rejects a shared controller, requires the Go image digest before creating the disposable database, starts the controller and Playwright in killable process groups, and drops the database during cleanup. Runner HTTP cleanup requests are individually time bounded; runner logs are redacted before capture. Real Go browser repair passed in1.8m (`.local/repair-browser-input-diagnostics.log`): isolated PostgreSQL/controller, enrolled runner, Gitea1.27.3, Qwen3:8b and gVisor. Verified390px layout, dialog focus, keyboard navigation, frozen baseline/target/candidate validation, artifact download and exactly one native publication. Input-dependent immutable tests expose expected/actual results; model repairs application source. Real dependency-upgrade controller acceptance passed153.769s (`.local/repair-upgrade-v3.log`). Earlier failures remain recorded in `t19-model-diagnosis.md`.

Live browser repair tests use `web/tests/helpers/repair-authority.ts` to provision local-only authority. Setup targets only an isolated disposable controller/database at `http://127.0.0.1:8081` and private Ollama endpoint `127.0.0.1:55435` through the fixture runner. Helper requires `REFORGE_ISOLATED_BROWSER_DATABASE=1` and refuses shared development controllers.

If isolated organisation policy or daily budget is absent, helper seeds a maintenance/v1 policy with every allow list empty and a zero-cap daily budget. Existing policy and budget versions are preserved. Helper adds only Go, selected local model, and its connection route, then restores prior versions and removes the model connection with version checks. Seeded safe baselines live only in disposable test database, which orchestration drops after run. No credentials are persisted by helper.
