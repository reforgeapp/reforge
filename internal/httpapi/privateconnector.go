package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/privateconnector"
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
		c.Next()
	})
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
