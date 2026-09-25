package httpapi

import (
	"github.com/gin-gonic/gin"

	"reforge/internal/autopilot"
)

func (s *Server) RegisterAutopilot(service *autopilot.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID/autopilot", s.IdentitySession())
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
		var input struct {
			Enabled bool `json:"enabled"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Put(c.Request.Context(), session, c.Param("orgID"), input.Enabled, version, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
}
