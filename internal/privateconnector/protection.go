package privateconnector

import (
	"net/http"
	"strings"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/forge"
)

type protectionHTTP struct{ client forge.HTTPClient }

func ProtectionHTTP(client forge.HTTPClient) forge.HTTPClient { return protectionHTTP{client} }

func (p protectionHTTP) Do(req *http.Request) (*http.Response, error) {
	if p.client == nil || req == nil || req.URL == nil || req.Method != http.MethodGet || !strings.HasSuffix(req.URL.Path, "/branch_protections") || req.URL.RawQuery != "" {
		return nil, auth.ErrForbidden
	}
	return p.client.Do(req)
}

func credentialEcho(raw []byte, c Connection) bool {
	return containsSecret(raw, c.Secret) || c.Protection != nil && containsSecret(raw, c.Protection.Secret)
}
