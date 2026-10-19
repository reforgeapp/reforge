package httpapi

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/reforgeapp/reforge/internal/alerts"
	"github.com/reforgeapp/reforge/internal/auth"
)

func (s *Server) RegisterAlerts(service *alerts.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID/alerts", s.IdentitySession())
	g.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Get(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.PUT("", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input alerts.Settings
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Put(c.Request.Context(), session, c.Param("orgID"), input, version)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.POST("/test", func(c *gin.Context) {
		var input alerts.Settings
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		err := service.Test(c.Request.Context(), session, c.Param("orgID"), input)
		switch {
		case errors.Is(err, alerts.ErrNotConfigured):
			Fail(c, 409, "email_unconfigured", "Enter a mail server host and from address", false)
		case errors.Is(err, auth.ErrForbidden), errors.Is(err, auth.ErrInvalid):
			IdentityFailure(c, err)
		case err != nil:
			Fail(c, 502, "email_failed", "The mail server rejected the message: "+err.Error(), false)
		default:
			c.Status(204)
		}
	})
}
