package privateconnector

import (
	"context"
	"net/http"
	"reforge/internal/auth"
	"reforge/internal/forge"
)

type guardedHTTP struct {
	client forge.HTTPClient
	check  func(context.Context) error
}

func GuardHTTP(client forge.HTTPClient, check func(context.Context) error) forge.HTTPClient {
	return guardedHTTP{client, check}
}
func (g guardedHTTP) Do(req *http.Request) (*http.Response, error) {
	if req.Method != "GET" && req.Method != "HEAD" && req.Method != "OPTIONS" {
		if g.check == nil {
			return nil, auth.ErrForbidden
		}
		if err := g.check(req.Context()); err != nil {
			return nil, err
		}
	}
	return g.client.Do(req)
}

func (g guardedHTTP) CloseIdleConnections() {
	if c, ok := g.client.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
