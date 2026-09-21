package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"reforge/internal/insights"
	"strconv"
	"time"
)

func insightFilter(c *gin.Context) (insights.Filter, bool) {
	f := insights.Filter{RepositoryID: c.Query("repository_id"), TeamID: c.Query("team_id"), Recipe: c.Query("recipe"), Provider: c.Query("provider"), ConnectionID: c.Query("connection_id"), State: c.Query("state"), ActorID: c.Query("actor_id"), Action: c.Query("action"), Cursor: c.Query("cursor"), Limit: 50}
	if v := c.Query("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 100 {
			Fail(c, 400, "invalid_filter", "Limit must be between 1 and 100", false)
			return f, false
		}
		f.Limit = n
	}
	for name, target := range map[string]**time.Time{"since": &f.Since, "until": &f.Until} {
		if v := c.Query(name); v != "" {
			at, e := time.Parse(time.RFC3339Nano, v)
			if e != nil {
				Fail(c, 400, "invalid_filter", "Time filters require RFC3339 timestamps", false)
				return f, false
			}
			*target = &at
		}
	}
	return f, true
}

func (s *Server) RegisterInsights(service *insights.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	g.GET("/usage", func(c *gin.Context) {
		f, ok := insightFilter(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		page, e := service.Usage(c.Request.Context(), session, c.Param("orgID"), f)
		if e != nil {
			IdentityFailure(c, e)
			return
		}
		c.JSON(200, page)
	})
	g.GET("/usage/summary", func(c *gin.Context) {
		f, ok := insightFilter(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		value, e := service.UsageSummary(c.Request.Context(), session, c.Param("orgID"), f)
		if e != nil {
			IdentityFailure(c, e)
			return
		}
		c.JSON(200, value)
	})
	g.GET("/overview", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, e := service.Overview(c.Request.Context(), session, c.Param("orgID"))
		if e != nil {
			IdentityFailure(c, e)
			return
		}
		c.JSON(200, value)
	})
	for _, export := range []bool{false, true} {
		path := "/audit-events"
		if export {
			path += "/export"
		}
		g.GET(path, func(c *gin.Context) {
			f, ok := insightFilter(c)
			if !ok {
				return
			}
			session, _ := SessionFromContext(c)
			page, e := service.Audit(c.Request.Context(), session, c.Param("orgID"), f)
			if e != nil {
				IdentityFailure(c, e)
				return
			}
			if !export {
				c.JSON(200, page)
				return
			}
			var buffer bytes.Buffer
			encoder := json.NewEncoder(&buffer)
			for _, event := range page.Items {
				if e = encoder.Encode(event); e != nil {
					IdentityFailure(c, e)
					return
				}
			}
			c.Header("Content-Disposition", `attachment; filename="audit-events.ndjson"`)
			c.Header("X-Export-Complete", strconv.FormatBool(page.Complete))
			if page.NextCursor != "" {
				c.Header("X-Next-Cursor", page.NextCursor)
			}
			c.Data(200, "application/x-ndjson", buffer.Bytes())
		})
	}
}
