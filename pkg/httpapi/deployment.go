package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Server) RegisterDeployment(service *deployment.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.GET("/repositories/:repoID/delivery-workflows", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Workflows(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		if out == nil {
			out = []forge.Workflow{}
		}
		c.JSON(200, gin.H{"items": out})
	})
	group.GET("/deployment-configurations", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Configurations(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"items": out})
	})
	group.PUT("/deployment-configurations/:environment", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var in deployment.Configuration
		if !identityJSON(c, &in) {
			return
		}
		if in.Environment != c.Param("environment") {
			Fail(c, 400, "invalid_request", "Environment must match the configuration route", false)
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.PutConfiguration(c.Request.Context(), session, c.Param("orgID"), in, version, c.GetString("request_id"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/deployment-configurations/:environment/track", func(c *gin.Context) {
		var in deployment.TrackRequest
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		out, err := service.Track(c.Request.Context(), session, c.Param("orgID"), c.Param("environment"), in, c.GetString("request_id"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(201, out)
	})
	group.POST("/deployment-configurations/:environment/preview", func(c *gin.Context) {
		var in deployment.PreviewRequest
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		out, err := service.Preview(c.Request.Context(), session, c.Param("orgID"), c.Param("environment"), in, c.GetString("request_id"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/deployments", func(c *gin.Context) {
		limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if err != nil {
			Fail(c, 400, "invalid_request", "Invalid deployment page size", false)
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.List(c.Request.Context(), session, c.Param("orgID"), c.Query("repository_id"), c.Query("environment"), c.Query("cursor"), limit)
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/deployments/:deploymentID/cancel", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		out, err := service.Cancel(c.Request.Context(), session, c.Param("orgID"), c.Param("deploymentID"), version, c.GetString("request_id"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.GET("/deployments/:deploymentID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("deploymentID"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/deployments", func(c *gin.Context) {
		var in struct {
			GateID         string `json:"gate_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		out, err := service.Request(c.Request.Context(), session, c.Param("orgID"), in.GateID, in.IdempotencyKey, c.GetString("request_id"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(201, out)
	})
	group.POST("/deployments/:deploymentID/observe", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		out, err := service.ObserveAs(c.Request.Context(), session, c.Param("orgID"), c.Param("deploymentID"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	s.Router.POST("/api/v1/deployment-health/:orgID/:deploymentID", func(c *gin.Context) {
		var in deployment.HealthReport
		if !identityJSON(c, &in) {
			return
		}
		err := service.ReceiveHealth(c.Request.Context(), c.Param("orgID"), c.Param("deploymentID"), in, c.GetHeader("X-Reforge-Signature"))
		if err != nil {
			deploymentFailure(c, err)
			return
		}
		c.Status(204)
	})
}

func deploymentFailure(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		Fail(c, 404, "not_found", "Deployment record not found", false)
		return
	}
	var provider *domain.ProviderError
	if errors.As(err, &provider) {
		Fail(c, 409, "deployment_native_unknown", "Native delivery evidence is unavailable or unqualified. Inspect the configured workflow, connection and qualification; reconcile any existing operation before retrying", false)
		return
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		Fail(c, 409, "deployment_environment_busy", "An operation already owns this environment or request key. Reconcile it before requesting another deployment", false)
		return
	}
	if errors.Is(err, privateconnector.ErrUnavailable) || errors.Is(err, privateconnector.ErrUnsupported) {
		Fail(c, 409, "deployment_transport_unavailable", "Reconnect the enrolled private runner and verify delivery adapter support", false)
		return
	}
	IdentityFailure(c, err)
}
