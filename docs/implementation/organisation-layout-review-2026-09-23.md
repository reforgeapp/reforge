# Organisation layout review — 2026-09-23

T29r locally complete. Baseline `4e10385`. Luna implemented; Astra reviewed and verified.

The team creation form centred its button against a combined label/input. Scoped form
alignment now places the button level with the input. A bounded desktop width prevents
unnecessary wrapping; narrow layouts keep both controls usable. No shared button, API,
query key, role, mutation or tenant behaviour changed.

Artifacts: `.local/organisation-layout-2026-09-23/`.

| Check | Result |
| --- | --- |
| `after/report.json`, six screenshots | Light/dark at 1440, 390 and 320px; input/button bottom offset 0px, Tab reaches button, no page overflow, axe violations or page errors |
| `browser.json` | Four existing Organisation/team-cache browser regressions pass; no skips |
| Production frontend build | Passed; existing chunk-size advisory remains |
| `container-build.log` and container smoke | Build passed; uid 10001, executable/migrations present, no Docker socket; packaged and served assets match |

First patch introduced desktop wrapping despite available space; rejected and corrected.
Captures retained under `rejected-wrap/`. Initial ad hoc axe harness used an implicit browser
context and failed; corrected to explicit context before final checks. Neither is reported
as a passing baseline. No new permanent test was added for the scoped CSS change.

Final assets: `index-Ck5lw7g7.js`, `index-Bd1oeqz3.css`.
Control image `reforge:gui-review`:
`sha256:6f1760aae4b317a7e0b994f1e924544a60e9031200d285c0fbe605deed3b69da`.

Existing demo remains `http://127.0.0.1:8080`; reload to obtain the bundle. No new application
stack or seed data. Owner handoff edit remains untouched. Broader review findings and open
work are tracked in [work-status.md](work-status.md); this layout check does not certify G5.
