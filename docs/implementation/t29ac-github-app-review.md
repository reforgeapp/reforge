# T29ac/T29ad GitHub App and forge onboarding review

2026-09-24. Narrow owner-authorized work; broader backlog remains paused. G5 has not passed.

## Implemented

- GitHub cloud guided setup: self-hosted manifest registration, hosted operator App installation, explicit OAuth PKCE, installation ownership and organisation-admin verification, repository discovery and selected import.
- Setup state binds organisation, user and original session; encrypted pending credentials, 15-minute expiry, single-use transitions, restart recovery and owner-role rechecks.
- Managed webhooks share normal inventory ingestion/deduplication. Every non-ping event checks installation identity. Hosted hooks use current operator signing secret; public per-tenant endpoints reject hosted connections.
- Provider-specific forge form: sensible API defaults, repository-URL rejection, token links, manual App RSA PEM upload/paste. Failed verification retains connection; replacement credentials update and retest that same connection.
- Repository picker imports visible selections only, persists sync identity across reload, and fences late responses after organisation changes. Forge details omit model/runtime fields. Add action follows active category.
- Hosted Compose overlay mounts secrets read-only. Guide documents distinct OAuth/setup callbacks, webhook, permissions and container file ownership.

## Review and verification

Astra reviewed worker changes; implementation by Opus5.5 and OpenCode DeepSeek4.1-Flash, maximum three workers, exclusive file ownership. No live provider mutations or paid Reforge API tests.

| Check | Actual result |
| --- | --- |
| Go build/vet and unit race suite | Passed |
| Full restricted PostgreSQL integration race suite | Passed; migrations001–037,55.1s |
| State-security regressions |10 passed; expiry cleanup cannot expose expired rows across tenants; wrong session does not consume setup |
| Managed webhook regressions |12 passed; rotation, old-secret rejection, foreign installation rejection, lifecycle/body-shape fencing, deduplication |
| API/SQL generation | Passed; prior OIDC SQL model drift regenerated and reviewed |
| Backend-connected browser |9 passed; real server/restricted PostgreSQL/isolated TLS GitHub fixture, zero browser API substitution |
| Restart | Actual server process restarted during pending setup; same session completed install/OAuth/import |
| Default browser suite |238 passed,29 opt-in skipped,0 failed |
| Fixture boundary/credential scan |0 unexpected requests;0 detected credential leaks |
| Browser layout |1440px/390px, light/dark; Escape and focus return exercised |
| Hosted Compose configuration |Valid dummy configuration passes; missing required App ID rejected |

Default-suite skips comprise nine GitHub connected cases run separately plus20 pre-existing opt-in scenarios. Fixture results demonstrate local implementation, not live GitHub certification.

Evidence: `.local/github-app-onboarding/state-final/`, `webhook-binding/`, `connected-final/`, `ui-final/`; worker reports alongside them. First failed attempts retained separately. Backend commit4f4d802; connected harness8e6c632; guide4f5e41f. GUI/demo source `87c3952`; final CSS spacing passed six capture checks after the default suite. Clean control and strict-MkDocs images built from git archive. Demo migration037 and restart preserved four connections,18 audit entries and the audit sentinel; JS/CSS match served image bytes. Protected pre-migration backup and final evidence: `.local/github-app-onboarding/demo-final/`. Launcher: `sh .local/opus-resume/demo/start.sh`; app http://127.0.0.1:8084, guide http://127.0.0.1:8082/docs/connections/.

## Remaining external actions

1. On a public HTTPS self-hosted installation, an owner completes GitHub manifest creation, grants requested permissions, selects repositories and finishes OAuth. Verify real inventory and signed webhook delivery, repository-selection updates, suspension/uninstall and reconnect.
2. Hosted operator supplies App identifiers, three mounted secrets and hosted KMS configuration; configures URLs/permissions from the connection guide. Certify install, cross-tenant binding refusal and secret rotation against that actual App.
3. Certify manual GitHub Enterprise Server App and real GitLab/Gitea token/import contracts using approved instances and credentials. No new live provider certification was attempted.

Loopback development App setup leaves webhooks inactive. Move to public HTTPS and recreate/reconfigure App before expecting inbound events. Existing revoked connections stay revoked; existing malformed endpoints are not silently rewritten. Credential fixes reuse a valid existing connection, while an invalid original endpoint requires a correctly configured new connection.
