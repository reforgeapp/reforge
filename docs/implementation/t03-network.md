# T03 fixed-origin connection transport

Status: local fixture verification complete. Enrolled-runner/private-provider certification remains T07/T11/T15/T27 work.

`network.ValidateEndpoint(endpoint, options)` validates syntax, literal addresses and route configuration without making a network request. `network.NewClient(endpoint, options)` returns a 30-second HTTP client restricted to that connection's exact scheme, authority and base path. Provider adapters append fixed operation paths and query parameters; they must not expose an arbitrary proxy operation.

Endpoint userinfo, query and fragment are rejected. Requests reject origin/port changes, Host overrides, path traversal including repeated percent encoding, proxy credentials and protocol upgrades. Allowed methods are GET, POST, PUT, PATCH, DELETE and HEAD. Encoded provider IDs such as GitLab `group%2Frepository` remain usable. Responses with redirects are returned unchanged; redirects are never followed.

Every new dial resolves the enrolled host, validates every returned address, then dials the selected literal IP. A mixed public/private DNS answer is rejected entirely. Existing connections remain pinned to their original validated destination. Public connections deny private, loopback, link-local, multicast, unspecified, metadata, shared-address, documentation and selected translation/tunnel ranges. IPv4-mapped IPv6 is checked as IPv4. HTTP proxy environment variables are ignored. TLS verifies the original hostname, requires TLS 1.2 or later and optionally appends the connection's CA PEM to system roots.

A private route requires nonempty organisation/connection/runner IDs, exact enrolled hostname, canonical explicit CIDRs, and `Options.RunnerID == PrivateRoute.RunnerID`. The connection service must load the route using its tenant/connection binding before constructing runner options. Empty runner identity rejects all private-route clients; the hosted control plane cannot use a private route. Route configuration is copied at client creation.

Private routes still reject metadata/link-local and reserved destinations. HTTP requires explicit development mode and an approved runner route resolving to a private address. Loopback additionally requires the exact endpoint hostname/address `127.0.0.1`, explicit CIDR inclusion and development mode; other loopback addresses and DNS aliases are rejected. Production connections require HTTPS.

Verified with `go test -race ./internal/network` and `go vet ./internal/network`: redirect rejection, origin/path/method confinement, literal-IP dialing, DNS rebinding and mixed-answer rejection, mapped IPv4/metadata addresses, private runner/CIDR matching, post-construction route mutation, custom CA trust, TLS hostname verification, and proxy-environment isolation. No external provider or customer network was contacted.

Implementation references: [Go HTTP transport](https://pkg.go.dev/net/http#Transport) and [OWASP SSRF prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html).
