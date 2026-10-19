package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/internal/policy"
	"strconv"
)

func (s *Server) RegisterPolicy(service *policy.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	g.GET("/policies/effective", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Resolve(c.Request.Context(), session, c.Param("orgID"), c.Query("repository_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.GET("/repositories/:repoID/policy", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Resolve(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.GET("/policies/versions", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		scope := policy.Scope{Kind: c.DefaultQuery("scope_kind", "organisation"), ID: c.DefaultQuery("scope_id", c.Param("orgID"))}
		page, err := service.ListVersions(c.Request.Context(), session, c.Param("orgID"), scope, limit, cursor)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, page)
	})
	g.POST("/policies/versions", func(c *gin.Context) {
		var input struct {
			Scope  policy.Scope  `json:"scope"`
			Policy policy.Policy `json:"policy"`
			Reason string        `json:"reason"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.CreateVersion(c.Request.Context(), session, c.Param("orgID"), input.Scope, input.Policy, input.Reason, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(201, value)
	})
	g.GET("/policies/versions/:versionID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.GetVersion(c.Request.Context(), session, c.Param("orgID"), c.Param("versionID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.POST("/policies/versions/:versionID/simulate", func(c *gin.Context) {
		var input struct {
			RepositoryID  string       `json:"repository_id"`
			PrimaryTeamID string       `json:"primary_team_id"`
			Input         policy.Input `json:"input"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Simulate(c.Request.Context(), session, c.Param("orgID"), c.Param("versionID"), input.RepositoryID, input.PrimaryTeamID, input.Input)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.POST("/policies/versions/:versionID/activate", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input struct {
			RepositoryID   string `json:"repository_id"`
			PrimaryTeamID  string `json:"primary_team_id"`
			SimulationHash string `json:"simulation_hash"`
			Reason         string `json:"reason"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Activate(c.Request.Context(), session, c.Param("orgID"), c.Param("versionID"), input.RepositoryID, input.PrimaryTeamID, version, input.SimulationHash, input.Reason, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(value, 10)))
		c.JSON(200, gin.H{"version": value})
	})
}
