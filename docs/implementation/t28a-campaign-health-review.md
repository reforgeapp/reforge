# Campaign health failure corpus

Revision under test: `78097ef` plus uncommitted `test/integration/campaign_health_corpus_test.go`.

## Local results

Disposable PostgreSQL integration tests ran with Go race detection:

```text
go test -race -count=1 ./test/integration -run '^(TestCampaign|TestDeploymentSignedProvenanceAndHealthContract|TestGitOps)' -v
PASS: all campaign tests, including missing health, restart replay, failed and unknown canaries, changed pins, pause fencing, expired approval, 1,000-member tenant fairness, pipeline health and repair resume.
PASS: deployment provenance and authenticated health contract.
PASS: GitOps configuration/RLS and signed health nonce replay/restart.
SKIP: TestGitOpsLiveProtectedPromotion requires disposable Gitea.
Package result: PASS, 31.703s; one opt-in Gitea test skipped.
```

New case removes verified-window evidence from an otherwise successful campaign execution. Controller records the canary as unknown, pauses campaign and does not dispatch later members. Restart case rebuilds campaign and workflow services over the same PostgreSQL rows; persisted canary action ID remains stable, replay observes prior success without repeating canary dispatch, then next bounded stage advances.

Existing corpus additionally checks fixed membership after selection mutation, unhealthy/unknown canaries, deployment completion without attributable health, authenticated health-window progression, changed evidence pins, and GitOps restart/replay.

## Limits

Execution adapters are local contract fixtures. No native forge approval or real pipeline was exercised. The disposable Gitea promotion scenario remained skipped because no Gitea endpoint was configured. Hosted tenant fairness, provider semantics and V15/V18 release qualification remain open; this report does not certify G4 or G5.

Confidence in these local PostgreSQL contract findings: 95%. Confidence in live provider behavior: not assessed.
