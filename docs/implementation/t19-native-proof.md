# T19 native commit proof

`TestNativeCommitProofAndPublication` is an opt-in integration check for the pinned disposable Gitea 1.27.3 fixture at `127.0.0.1:53000`. Set `REFORGE_GITEA_TEST=1` and `REFORGE_TEST_ROOT` to the repository root containing `.local/gitea/reforge-admin.token` and `.local/gitea/reforge-bot.token`.

The test creates a private repository, records the baseline commit, and uses the Gitea adapter with a mandatory application branch authorizer. It verifies that the source-only branch commit has the exact returned SHA, exactly the baseline parent, and the operation marker. It then reads the immutable source manifest at that SHA and checks completeness and the immutable-ref proof. Native publication is repeated with the same operation ID; the adapter must return the existing pull request and `FindChangeByOperation` must reconcile the exact head and target SHAs.

Run:

```text
REFORGE_GITEA_TEST=1 REFORGE_TEST_ROOT=/home/mnorris/repos/reforge go test ./test/integration -run TestNativeCommitProofAndPublication -count=1 -v
```

The test skips when opt-in is absent, credentials are absent, or the local fixture is unavailable. Tokens are never logged.
