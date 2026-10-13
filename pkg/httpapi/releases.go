package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/delivery"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/release"
	"strconv"
)

func (s *Server) RegisterReleases(service *release.Service, commands *delivery.Service) {
	s.Router.POST("/api/v1/orgs/:orgID/repositories/:repoID/release-reports", func(c *gin.Context) {
		var in deployment.SignedProvenance
		if !identityJSON(c, &in) {
			return
		}
		out, err := service.Receive(c.Request.Context(), c.Param("orgID"), c.Param("repoID"), in)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(201, out)
	})
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.GET("/repositories/:repoID/deployment-setup", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := commands.Discover(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/repositories/:repoID/releases", func(c *gin.Context) {
		limit, err := strconv.Atoi(c.DefaultQuery("limit", "30"))
		if err != nil {
			Fail(c, 400, "invalid_request", "Invalid release page size", false)
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.List(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), c.Query("cursor"), limit)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/release-receipts/:receiptID/deploy", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := commands.Deploy(c.Request.Context(), session, c.Param("orgID"), c.Param("receiptID"), c.GetString("request_id"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(201, out)
	})
	for _, kind := range []string{"native", "gitops"} {
		group.POST("/delivery/"+kind+"/:operationID/restore/preview", func(c *gin.Context) {
			version, ok := identityVersion(c)
			if !ok {
				return
			}
			session, _ := SessionFromContext(c)
			out, err := commands.PreviewRestore(c.Request.Context(), session, c.Param("orgID"), kind, c.Param("operationID"), version, c.GetString("request_id"))
			if err != nil {
				deploymentFailure(c, err)
				return
			}
			c.JSON(200, out)
		})
		group.POST("/delivery/"+kind+"/:operationID/restore", func(c *gin.Context) {
			version, ok := identityVersion(c)
			if !ok {
				return
			}
			var in struct {
				GateID string `json:"gate_id"`
			}
			if !identityJSON(c, &in) {
				return
			}
			session, _ := SessionFromContext(c)
			out, err := commands.ConfirmRestore(c.Request.Context(), session, c.Param("orgID"), kind, c.Param("operationID"), in.GateID, version, c.GetString("request_id"))
			if err != nil {
				deploymentFailure(c, err)
				return
			}
			c.JSON(201, out)
		})
	}
}
