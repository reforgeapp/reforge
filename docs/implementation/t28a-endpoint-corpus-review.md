# T28a private endpoint corpus

Status: local fixture coverage added; V04 remains unqualified for enrolled hosted or self-hosted runners.

`internal/privateconnector/endpoint_corpus_test.go` exercises the production `validateConnection` grant boundary against malformed origins, credential-bearing URLs, query/fragment input, encoded traversal, invalid ports, plaintext remote URLs, mismatched approved routes, loopback/private literals and common cloud metadata addresses. A valid development-only approved loopback Gitea route runs through `Executor.Execute`; its redirect to a separate metadata-style fixture is returned as a provider failure and never reaches that second server.

Existing `internal/network/client_test.go` fixture-resolver checks remain the DNS evidence: `TestDNSRebindingAndLiteralDial` proves a later changed answer and mixed unsafe answers cannot reach the dialer; `TestPrivateDNSCIDRAndRouteSnapshot` proves private CIDR changes and special addresses cannot use a private exception. `TestEndpointAndPrivateRouteBoundaries`, `TestRequestOriginPathAndMethodGuard` and `TestLocalRedirectAndCustomCA` cover broader special-address, origin/path, proxy and redirect behavior.

Checks run:

- `go test -race ./internal/privateconnector -run 'Test(EndpointHostileCorpusRejectedAtGrantBoundary|ExecutorDoesNotFollowApprovedRouteRedirectToMetadata)$' -count=1` — passed.
- `go test -race ./internal/network -run 'Test(EndpointAndPrivateRouteBoundaries|DNSRebindingAndLiteralDial|RequestOriginPathAndMethodGuard|LocalRedirectAndCustomCA|PrivateDNSCIDRAndRouteSnapshot)$' -count=1` — passed.

This corpus uses local fixtures and injected resolver/dialer functions only. It does not exercise actual DNS rebinding against an enrolled runner, cloud metadata services, hosted network policy, production DNS, live forge/model endpoints, or provider-issued redirects. No hosted isolation claim follows; V04 and G5 stay open pending runner-host certification.
