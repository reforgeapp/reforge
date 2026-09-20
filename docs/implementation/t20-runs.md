# T20 Runs

Runs exposes persisted workflow tasks and repair evidence in the organisation route. The list uses the server cursor and deep-links the selected task in the URL. Detail loads the repair record, displays frozen head, plan, policy, validation output, plaintext patches and server-authorized artifact download links.

The event stream is advisory and reconnects after transport failure. Reset and access-revoked events reload authoritative task state and expose an actionable status. Task terminal state remains separate from the native change state. Cancel and resume send the current quoted version with the session CSRF token.

Browser coverage uses explicit route fixtures for keyboard-accessible list/detail navigation. It does not claim live repair execution acceptance.

Focused browser coverage verifies escaped hostile plaintext, frozen command display, narrow keyboard deep links, quoted version and CSRF mutation headers, event deduplication, and access-revoked stream handling. Fixtures are explicit and do not represent live repair acceptance.

Run evidence keeps validation stages distinct (H baseline, H+patch, T+patch, native C), renders exact test cases and server-derived plain text, and preserves source-authorized links. Finding details expose bot ownership, dependency and check evidence with bounded previews and validated external links.
