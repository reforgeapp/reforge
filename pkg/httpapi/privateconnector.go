package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Server) RegisterPrivateConnector(connector *privateconnector.Connector) {
	group := s.Router.Group("/runner/v1/private", func(c *gin.Context) {
		origin, e := url.Parse(s.Config.PublicURL)
		value := c.GetHeader("Authorization")
		if connector == nil || e != nil || c.Request.Host != origin.Host || c.GetHeader("Origin") != "" || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			IdentityFailure(c, auth.ErrForbidden)
			return
		}
		if !strings.HasPrefix(value, "Bearer ") || len(value) > 263 {
			IdentityFailure(c, auth.ErrUnauthenticated)
			return
		}
		c.Set("private_credential", strings.TrimPrefix(value, "Bearer "))
		c.Header("Cache-Control", "no-store")
		if c.GetHeader(forwardedHeader) != "" {
			c.Request = c.Request.WithContext(privateconnector.WithForwarded(c.Request.Context()))
		}
		c.Next()
	})
	toOwner := func(c *gin.Context, limit int64) bool {
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, limit+1))
		if err != nil || int64(len(raw)) > limit {
			IdentityFailure(c, auth.ErrInvalid)
			return true
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		var in struct {
			GrantID string `json:"grant_id"`
		}
		if c.GetHeader(forwardedHeader) != "" || json.Unmarshal(raw, &in) != nil || connector.Holds(in.GrantID) {
			return false
		}
		address, err := connector.GrantOwner(c.Request.Context(), c.GetString("private_credential"), in.GrantID)
		if err != nil || address == "" {
			return false
		}
		s.forwardPrivate(c, address, raw, 2*time.Minute)
		return true
	}
	group.POST("/poll", func(c *gin.Context) {
		if strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
			Fail(c, 415, "unsupported_media_type", "JSON required", false)
			return
		}
		_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(5 * time.Second))
		raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 1025))
		if e != nil || len(raw) > 1024 || strings.TrimSpace(string(raw)) != "{}" {
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		grant, e := connector.Poll(c.Request.Context(), c.GetString("private_credential"))
		var elsewhere privateconnector.Elsewhere
		if errors.As(e, &elsewhere) {
			s.forwardPrivate(c, elsewhere.Address, raw, privateconnector.MaxTTL+30*time.Second)
			return
		}
		if errors.Is(e, privateconnector.ErrUnavailable) {
			c.Status(204)
			return
		}
		if e != nil {
			privateFailure(c, e)
			return
		}
		raw, e = grant.MarshalWire()
		if e != nil || len(raw) > privateconnector.MaxGrant {
			privateFailure(c, privateconnector.ErrInvalid)
			return
		}
		c.Data(200, "application/json", raw)
	})
	group.POST("/status", func(c *gin.Context) {
		if toOwner(c, 4096) {
			return
		}
		var in struct {
			GrantID string `json:"grant_id"`
		}
		if !identityJSON(c, &in) {
			return
		}
		if err := connector.Active(c.Request.Context(), c.GetString("private_credential"), in.GrantID, c.GetHeader("X-Private-Result-Capability")); err != nil {
			privateFailure(c, err)
			return
		}
		c.Status(204)
	})
	group.POST("/results", func(c *gin.Context) {
		_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(5 * time.Second))
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, privateconnector.MaxResponse)
		if toOwner(c, privateconnector.MaxResponse) {
			return
		}
		var in privateconnector.Completion
		if !identityJSON(c, &in) {
			return
		}
		if e := connector.Complete(c.Request.Context(), c.GetString("private_credential"), in.GrantID, c.GetHeader("X-Private-Result-Capability"), in.Result); e != nil {
			privateFailure(c, e)
			return
		}
		c.Status(204)
	})
}

const forwardedHeader = "X-Reforge-Forwarded"

var privateForwarder = &http.Client{Transport: &http.Transport{Proxy: nil}}

func (s *Server) forwardPrivate(c *gin.Context, address string, body []byte, timeout time.Duration) {
	origin, err := url.Parse(s.Config.PublicURL)
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	request, requestErr := http.NewRequestWithContext(ctx, c.Request.Method, "http://"+address+c.Request.URL.Path, bytes.NewReader(body))
	if err != nil || requestErr != nil {
		privateFailure(c, privateconnector.ErrUnavailable)
		return
	}
	request.Host = origin.Host
	for _, name := range []string{"Authorization", "Content-Type", "X-Private-Result-Capability"} {
		if value := c.GetHeader(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	request.Header.Set(forwardedHeader, "1")
	response, err := privateForwarder.Do(request)
	if err != nil {
		privateFailure(c, privateconnector.ErrUncertain)
		return
	}
	defer response.Body.Close()
	if value := response.Header.Get("Content-Type"); value != "" {
		c.Header("Content-Type", value)
	}
	c.Status(response.StatusCode)
	_, _ = io.Copy(c.Writer, io.LimitReader(response.Body, privateconnector.MaxResponse))
}

func privateFailure(c *gin.Context, e error) {
	switch {
	case errors.Is(e, privateconnector.ErrConflict):
		Fail(c, 409, "private_busy", "Private operation is already active or consumed", false)
	case errors.Is(e, privateconnector.ErrUnsupported):
		Fail(c, 409, "private_unsupported", "Private operation is not registered", false)
	case errors.Is(e, privateconnector.ErrInvalid):
		IdentityFailure(c, auth.ErrInvalid)
	case errors.Is(e, privateconnector.ErrUncertain):
		Fail(c, 409, "private_uncertain", "Private operation requires reconciliation", false)
	default:
		IdentityFailure(c, e)
	}
}
