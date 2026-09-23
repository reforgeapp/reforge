# T29u qwen2.5:7b protected-merge review

Date: 2026-09-24

**Result: passed once, first attempt, no retries.** `TestRepairLiveProtectedMerge` passed in 81.74s (wrapper 83.72s, exit 0) using genuine local `qwen2.5:7b` (Ollama 0.34.2, manifest SHA-256 `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`, model layer `sha256:2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730`, 7.6B Q4_K_M, `tools` capability). Fixture, route caps (1,024 output tokens, 180s, one request per turn), organisation budget, policy, Gitea protection and gVisor limits were unchanged. Test source hashes match the qwen3:4b run; HEAD was `c6603a9` with no Go/test/script change since `fe67744`.

The model used five tool turns: read `value.js`, read target `value.js` twice, read `value.test.js`, then applied a source-only `value.js` patch (`a - b` → `a + b`). The frozen test was not modified. The test asserted reproduced baseline, verified candidate checks, a non-empty diff and candidate artifacts, then exactly one native PR whose head is the candidate.

Protected merge evidence, all asserted by the test against Gitea 1.27.3:

| Item | Value |
| --- | --- |
| Native PR | `1` |
| Baseline | `44bf2654ab9ff028adaf16b3d99914bbeeed6e44` |
| Candidate / canonical merge SHA | `67955e97975e38b1bb431b155e7d01fdf6f7e2b2` |
| Merge operation | `2a62fc4f-fa73-4404-a25c-9da38691a210` |
| Method | fast-forward-only |

Before approval, the gate was not `allow` and a merge request on it was rejected. Protection required one approval, dismissed stale approvals and blocked outdated branches, rejected reviews, official review requests and admin override. A separate reviewer approved the exact candidate head; the refreshed gate allowed. A request against a gate made stale by a configuration version change returned conflict. The authorized request merged; the operation's native merge SHA equalled the candidate. After injected loss of native result, a fresh service re-observed the same canonical SHA. Idempotent replay returned the same operation, and a cross-tenant read was denied.

Setup: after confirming no active guest, runner, Ollama or Gitea processes, `qwen3:8b` manifest and model metadata were preserved, then only `qwen3:8b` was removed with `ollama rm`. qwen3:1.7b and qwen3:4b were retained. The official Ollama pull took 358s. Free space after the pull was 4,020,641,792 bytes, above the 3 GiB floor. The model was loaded with an empty-prompt `keep_alive` request before the test; this did not change test inputs or limits. `/reforge_test` on PostgreSQL `127.0.0.1:55437` was recreated with zero clients and the repository guard passed. Migration and runtime grants were then applied. `/reforge_dev` was not touched. Gitea's fixture repository was deleted by test cleanup; pre-existing repositories were unchanged. Ollama and Gitea were stopped afterward, while demo and PostgreSQL containers kept running.

Evidence is under `.local/opus-resume/positive-merge-qwen25/`, including `protected-merge-test.log`, `run.json`, `run-test.sh`, `source-identity.json`, the model manifest and digest, pull log, removed-model metadata, preflight/post-test process checks and service logs. A secret scan found no fixture tokens or DB passwords.

## Boundary

This is one local service integration pass. Merge qualification is seeded in-process from the local Gitea contract digest; this does not prove an operator qualification workflow, model qualification across runs, the GUI journey, or provider/G5 certification. The fixture is JavaScript; the browser repair fixture is Go and is unverified with this model.

## Positive GUI bridge (read-only finding)

Gitea required status contexts always evaluate `native_rules` as unknown (`internal/forge/gitea/protection.go`). Gate `validation` on Gitea can therefore only come from a published, validated `repair_runs` row for the exact change head (`internal/mergecontrol/inspect.go`). The fixture PR in `connected-reviewer.spec.ts` can never pass this gate. The positive GUI journey must merge a PR published by a Reforge repair.

Minimal bridge: one new opt-in Playwright spec that reuses `helpers/repair-fixture.ts` and `helpers/repair-authority.ts` from `repair-live.spec.ts` on an isolated DB/server (with `REFORGE_LIVE_REPAIR_MODEL=qwen2.5:7b`), then continues after publication:

1. Native fixture setup through the Gitea API, as in `verifyLiveProtectedMerge`: reviewer write collaborator, allow fast-forward-only, and `main` protection with one approval, stale dismissal, outdated/rejected/official-request blocking, admin override blocked and status checks off.
2. GUI: rotate the forge connection to the reviewer merge-actor token and test it; create and test the inspector connection until both are healthy.
3. GUI Repositories → Maintenance: set merge authority to `reforge`.
4. GUI Policies: create a version that adds `fast-forward-only` to merge methods, then simulate a merge and activate it.
5. GUI Merge settings: enable, select the inspector, and set a cooperation reference. Record qualification with a reference, the `test/forge/gitea/contract_test.go` SHA-256, verified/expiry times, and exact head plus strict target.
6. Record a native reviewer `APPROVED` review on the candidate head through the Gitea API as an external actor.
7. GUI: refresh the merge preview and require `allow`. Request the merge, wait for `merged`, and reload. Assert that the operation merge SHA equals the PR head and the Gitea `main` SHA.

Prerequisites: build `bin/reforge-runner` and the server from current HEAD, provide the runner's gVisor runtime config, and run Ollama with qwen2.5:7b plus Gitea 53000. qwen2.5:7b must also repair the Go fixture within the same caps; if it fails, report that without relaxing limits. Qualification in step 5 is still operator-recorded local contract evidence, not certification.
