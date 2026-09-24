package httpapi

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"reforge/internal/connections"
	"reforge/internal/network"
	"reforge/internal/privateconnector"
	"reforge/internal/secrets"
)

func (s *Server) RegisterConnections(service *connections.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID/connections", s.IdentitySession())
	for path, kind := range map[string]string{"forges": "forge", "models": "model", "agents": "agent", "delivery": "delivery"} {
		g.GET("/"+path, func(c *gin.Context) {
			limit, cursor, ok := identityPage(c)
			if !ok {
				return
			}
			session, _ := SessionFromContext(c)
			page, err := service.List(c.Request.Context(), session, c.Param("orgID"), kind, limit, cursor)
			if err != nil {
				connectionFailure(c, err)
				return
			}
			c.JSON(200, page)
		})
		g.POST("/"+path, func(c *gin.Context) {
			var input connections.CreateRequest
			if !identityJSON(c, &input) {
				return
			}
			input.Kind = kind
			session, _ := SessionFromContext(c)
			value, err := service.Create(c.Request.Context(), session, c.Param("orgID"), input, c.GetString("request_id"))
			input.Secret = ""
			if err != nil {
				connectionFailure(c, err)
				return
			}
			connectionResponse(c, 201, value)
		})
	}
	g.POST("/model-catalog", func(c *gin.Context) {
		var input connections.CatalogRequest
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		items, err := service.ModelCatalog(c.Request.Context(), session, c.Param("orgID"), input)
		input.Secret = ""
		if err != nil {
			connectionFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"items": items})
	})
	g.GET("", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		page, err := service.List(c.Request.Context(), session, c.Param("orgID"), c.Query("kind"), limit, cursor)
		if err != nil {
			connectionFailure(c, err)
			return
		}
		c.JSON(200, page)
	})
	g.POST("", func(c *gin.Context) {
		var input connections.CreateRequest
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Create(c.Request.Context(), session, c.Param("orgID"), input, c.GetString("request_id"))
		input.Secret = ""
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 201, value)
	})
	g.GET("/:connectionID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 200, value)
	})
	g.POST("/:connectionID/test", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Test(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, c.GetString("request_id"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 200, value)
	})
	g.POST("/:connectionID/rewrap", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Rewrap(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, c.GetString("request_id"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 200, value)
	})
	g.POST("/:connectionID/rotate", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input struct {
			Secret string `json:"secret"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Rotate(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, input.Secret, c.GetString("request_id"))
		input.Secret = ""
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 200, value)
	})
	g.DELETE("/:connectionID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Revoke(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, c.GetString("request_id"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 200, value)
	})
	g.PUT("/:connectionID/private-route", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input struct {
			Route *connections.Route `json:"route"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.SetRoute(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), version, input.Route, c.GetString("request_id"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		connectionResponse(c, 200, value)
	})
}
func connectionResponse(c *gin.Context, status int, value connections.Connection) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	c.JSON(status, value)
}
func connectionFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, privateconnector.ErrUnavailable):
		Fail(c, 503, "private_unavailable", "Start the enrolled connector and route its polling and this request to the same controller instance", true)
	case errors.Is(err, privateconnector.ErrUncertain):
		Fail(c, 409, "private_uncertain", "The private capability probe did not finish; verify runner connectivity and retry this read-only test", true)
	case errors.Is(err, secrets.ErrKMSUnavailable), errors.Is(err, secrets.ErrInvalid):
		Fail(c, 503, "encryption_unavailable", "Credential encryption unavailable; ask the operator to verify wrapping keys, AWS credentials and KMS permissions", true)
	case errors.Is(err, connections.ErrRunnerRequired):
		Fail(c, 409, "runner_required", "Enrol an authorised runner before approving a private route", false)
	case errors.Is(err, connections.ErrCatalogUnavailable):
		Fail(c, 502, "catalog_unavailable", "Model list unavailable; check endpoint, credential and provider access, then retry", true)
	case errors.Is(err, connections.ErrRevoked):
		Fail(c, 409, "connection_revoked", "Connection revoked; create a new connection", false)
	case errors.Is(err, network.ErrDestination), errors.Is(err, network.ErrRequest):
		Fail(c, 400, "destination_denied", "Destination blocked; configure an approved private route and enrolled runner where required", false)
	default:
		IdentityFailure(c, err)
	}
}
