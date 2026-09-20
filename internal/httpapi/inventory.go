package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"reforge/internal/auth"
	"reforge/internal/inventory"
)

func (s *Server) RegisterInventory(service *inventory.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.POST("/inventory-syncs", func(c *gin.Context) {
		var input inventory.SyncInput
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		j, err := service.StartSync(c.Request.Context(), session, c.Param("orgID"), input, c.GetString("request_id"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		inventoryJob(c, 202, j)
	})
	group.GET("/inventory-syncs", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Jobs(c.Request.Context(), session, c.Param("orgID"), limit, cursor)
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/inventory-syncs/:syncID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		j, err := service.Job(c.Request.Context(), session, c.Param("orgID"), c.Param("syncID"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		inventoryJob(c, 200, j)
	})
	group.DELETE("/inventory-syncs/:syncID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		j, err := service.Cancel(c.Request.Context(), session, c.Param("orgID"), c.Param("syncID"), version, c.GetString("request_id"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		inventoryJob(c, 200, j)
	})
	group.GET("/inventory-syncs/:syncID/candidates", func(c *gin.Context) {
		limit, cursor, ok := inventoryPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Candidates(c.Request.Context(), session, c.Param("orgID"), c.Param("syncID"), limit, cursor)
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/inventory-syncs/:syncID/import", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input inventory.ImportInput
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		j, err := service.StartImport(c.Request.Context(), session, c.Param("orgID"), c.Param("syncID"), input, version, c.GetString("request_id"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		inventoryJob(c, 202, j)
	})
	group.GET("/repositories", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.RepositoriesFiltered(c.Request.Context(), session, c.Param("orgID"), limit, cursor, inventory.RepositoryFilter{Query: c.Query("q"), Provider: c.Query("provider"), TeamID: c.Query("team_id"), Status: c.Query("status")})
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/repositories/:repoID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Repository(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/repositories/:repoID/changes", func(c *gin.Context) {
		limit, cursor, ok := inventoryPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Changes(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), limit, cursor)
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/connections/:connectionID/webhook", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Webhook(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(out.Version, 10)))
		c.JSON(200, out)
	})
	group.PUT("/connections/:connectionID/webhook", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.ConfigureWebhook(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, c.GetString("request_id"))
		if err != nil {
			inventoryFailure(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("ETag", strconv.Quote(strconv.FormatInt(out.Version, 10)))
		c.JSON(200, out)
		out.Secret = ""
	})
	group.DELETE("/connections/:connectionID/webhook", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		if err := service.RevokeWebhook(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, c.GetString("request_id")); err != nil {
			inventoryFailure(c, err)
			return
		}
		c.Status(204)
	})
	s.Router.POST("/hooks/v1/:orgID/:endpointID", func(c *gin.Context) {
		if err := s.Auth.CheckRequest(&http.Request{Method: "GET", Host: c.Request.Host}, ""); err != nil {
			IdentityFailure(c, err)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, inventory.MaxWebhookBytes))
		if err != nil {
			Fail(c, 413, "payload_too_large", "Webhook body exceeds limit", false)
			return
		}
		if err = service.HandleWebhook(c.Request.Context(), c.Param("orgID"), c.Param("endpointID"), c.Request.Header, body); err != nil {
			inventoryFailure(c, err)
			return
		}
		c.Status(202)
	})
}
func inventoryJob(c *gin.Context, status int, j inventory.Job) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(j.Version, 10)))
	c.JSON(status, j)
}
func inventoryFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, inventory.ErrStale), errors.Is(err, inventory.ErrIncomplete):
		Fail(c, 409, "inventory_stale", "Verify the connection and start a new complete inventory sync", false)
	case errors.Is(err, inventory.ErrBusy):
		Fail(c, 429, "inventory_busy", "Inventory queue is full; retry later", true)
	default:
		IdentityFailure(c, err)
	}
}
func inventoryPage(c *gin.Context) (int, string, bool) {
	limit := 100
	var err error
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 200 || len(c.Query("cursor")) > 256 {
		IdentityFailure(c, auth.ErrInvalid)
		return 0, "", false
	}
	return limit, c.Query("cursor"), true
}
