package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/reforgeapp/reforge/pkg/autopilot"
)

func (s *Server) RegisterAutopilot(service *autopilot.Service) {
	repositories := s.Router.Group("/api/v1/orgs/:orgID/repositories", s.IdentitySession())
	repositories.GET("/:repoID/maintenance-status", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.MaintenanceStatus(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	for _, preview := range []bool{true, false} {
		path := "/:repoID/set-up-maintenance"
		if preview {
			path += "/preview"
		}
		repositories.POST(path, func(c *gin.Context) {
			var input autopilot.MaintenanceCommand
			if !identityJSON(c, &input) {
				return
			}
			session, _ := SessionFromContext(c)
			var value autopilot.MaintenancePlan
			var err error
			if preview {
				value, err = service.PreviewMaintenance(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), input)
			} else {
				value, err = service.ConfigureMaintenance(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), input, c.GetString("request_id"))
			}
			if err != nil {
				IdentityFailure(c, err)
				return
			}
			c.JSON(200, value)
		})
	}
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
	g.GET("/needs", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Needs(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.GET("/metrics", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Metrics(c.Request.Context(), session, c.Param("orgID"))
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
	s.Router.Group("/api/v1/orgs/:orgID/repositories", s.IdentitySession()).POST("/:repoID/run", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.Request(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), c.GetString("request_id")); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Status(204)
	})
}
