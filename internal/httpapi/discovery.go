package httpapi

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/inventory"
)

func (s *Server) RegisterDiscovery(service *discovery.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	g.GET("/findings", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		page, err := service.List(c.Request.Context(), session, c.Param("orgID"), limit, cursor, discovery.Filter{
			Query:        c.Query("q"),
			RepositoryID: c.Query("repository_id"),
			State:        c.Query("state"),
			Category:     c.Query("category"),
			Severity:     c.Query("severity"),
		})
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		c.JSON(200, page)
	})
	g.GET("/findings/:findingID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		finding, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("findingID"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 200, finding.Version, finding)
	})
	g.PATCH("/findings/:findingID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input discovery.Update
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		finding, err := service.Update(c.Request.Context(), session, c.Param("orgID"), c.Param("findingID"), input, version, c.GetString("request_id"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 200, finding.Version, finding)
	})
	g.POST("/findings/advisories", func(c *gin.Context) {
		var input discovery.AdvisoryInput
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		finding, err := service.ImportAdvisory(c.Request.Context(), session, c.Param("orgID"), input, c.GetString("request_id"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 201, finding.Version, finding)
	})
	g.GET("/repositories/:repoID/maintenance", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		config, err := service.GetConfig(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 200, config.Version, config)
	})
	g.PUT("/repositories/:repoID/maintenance", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input discovery.Config
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		config, err := service.PutConfig(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), input, version, c.GetString("request_id"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 200, config.Version, config)
	})
	g.GET("/repositories/:repoID/discovery", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		scan, err := service.ScanStatus(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 200, scan.Version, scan)
	})
	g.POST("/repositories/:repoID/discovery", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		scan, err := service.StartScan(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), c.GetString("request_id"))
		if err != nil {
			discoveryFailure(c, err)
			return
		}
		discoveryResponse(c, 202, scan.Version, scan)
	})
}

func discoveryResponse(c *gin.Context, status int, version int64, value any) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(version, 10)))
	c.JSON(status, value)
}

func discoveryFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, discovery.ErrStale), errors.Is(err, inventory.ErrStale):
		Fail(c, 409, "discovery_stale", "Discovery evidence changed; refresh the finding and retry", false)
	case errors.Is(err, discovery.ErrDuplicate):
		Fail(c, 409, "discovery_duplicate", "Overlapping maintenance work exists; review the existing change", false)
	case errors.Is(err, discovery.ErrBlocked):
		Fail(c, 409, "discovery_blocked", "Finding requires reviewed ownership or complete evidence", false)
	default:
		IdentityFailure(c, err)
	}
}
