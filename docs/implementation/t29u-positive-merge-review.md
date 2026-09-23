# T29u positive protected merge

Date: 2026-09-23

**Result: blocked; no positive GUI merge claimed.** Connected browser acceptance currently proves the negative path: after a real current-head Gitea approval, the merge gate remains unknown and Reforge returns 409 while qualification and validation evidence are absent. See [connected reviewer journey](t28a-reviewer-journey-review.md).

The repository contains `TestRepairLiveProtectedMerge` in `test/integration/repair_live_test.go`. It runs a real repair through Ollama and gVisor, records validated candidate evidence, configures policy and merge qualification, obtains a native Gitea approval, requests merge through Reforge services, and verifies the canonical native merge. It is a Go integration test over in-process services, not a browser-connected GUI journey. The existing browser test does not reach a qualified merge.

Local preflight on 2026-09-23 found the active browser run using `127.0.0.1:8094` and PostgreSQL `127.0.0.1:55435`; these services were left untouched. No Ollama executable, process, container image, or listener was available. No Gitea container or listener was available. The protected-merge Go test expects Ollama at `127.0.0.1:55435`, Gitea at `127.0.0.1:53000`, and explicit disposable test database URLs naming `/reforge_test`. Pinned gVisor and the sandbox helper are present. No large model was downloaded and no paid API was used.

Do not fill qualification, policy, approval, or validation fields to make this pass. Qualification must bind the observed Gitea version and current connection/inspector versions; validation must come from a completed run; native approval must target the current candidate; the protected merge must be requested and observed through Reforge. The gate is correct to remain closed without those inputs.

To finish local service acceptance, wait until the current browser run releases port 55435; provide an approved, locally installed `qwen3:1.7b` Ollama runtime on that port and disposable Gitea 1.27.3 on port 53000 with the existing local fixture accounts. Set `REFORGE_TEST_DATABASE_URL` and `REFORGE_TEST_MIGRATION_DATABASE_URL` to the same disposable PostgreSQL database named `/reforge_test`, run `python3 scripts/check-test-database.py`, then apply migrations and run:

```sh
REFORGE_MIGRATION_DATABASE_URL="$REFORGE_TEST_MIGRATION_DATABASE_URL" go run ./cmd/migrate
REFORGE_LIVE_REPAIR_TEST=1 \
REFORGE_TEST_ROOT="$PWD" \
REFORGE_REPAIR_TEST_MODEL=qwen3:1.7b \
go test -count=1 -v ./test/integration -run '^TestRepairLiveProtectedMerge$'
```

That run certifies the real repair-to-protected-merge service path only. T29u still needs a connected browser/API run against the same completed validated change, with native approval and canonical merge SHA observed after reload, before positive GUI acceptance can pass.
