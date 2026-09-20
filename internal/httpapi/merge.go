package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"reforge/internal/mergecontrol"
	"reforge/internal/privateconnector"
)

func (s *Server) RegisterMerge(service *mergecontrol.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.GET("/repositories/:repoID/merge-configuration", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Config(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			mergeFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.PUT("/repositories/:repoID/merge-configuration", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var in mergecontrol.Configuration
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.PutConfig(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), in, version, c.GetString("request_id"))
		if err != nil {
			mergeFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/repositories/:repoID/changes/:changeID/merge-preview", func(c *gin.Context) {
		var in struct {
			Method string `json:"method"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Inspect(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"), c.Param("changeID"), in.Method, c.GetString("request_id"))
		if err != nil {
			mergeFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	group.POST("/merge-operations", func(c *gin.Context) {
		var in struct {
			GateID         string `json:"gate_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Request(c.Request.Context(), session, c.Param("orgID"), in.GateID, in.IdempotencyKey, c.GetString("request_id"))
		if err != nil {
			mergeFailure(c, err)
			return
		}
		c.JSON(201, out)
	})
	group.GET("/merge-operations/:operationID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("operationID"))
		if err != nil {
			mergeFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	for _, action := range []string{"reconcile", "cancel"} {
		group.POST("/merge-operations/:operationID/"+action, func(c *gin.Context) {
			version, ok := identityVersion(c)
			if !ok {
				return
			}
			session, _ := SessionFromContext(c)
			fn := service.Reconcile
			if action == "cancel" {
				fn = service.Cancel
			}
			out, err := fn(c.Request.Context(), session, c.Param("orgID"), c.Param("operationID"), version, c.GetString("request_id"))
			if err != nil {
				mergeFailure(c, err)
				return
			}
			c.JSON(200, out)
		})
	}
}

func mergeFailure(c *gin.Context, err error) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		Fail(c, 409, "merge_target_busy", "A merge is already active for this target; reconcile it before starting another", false)
		return
	}
	if errors.Is(err, privateconnector.ErrUnavailable) {
		Fail(c, 409, "runner_unavailable", "Reconnect the enrolled private runner before inspecting native merge evidence", false)
		return
	}
	if errors.Is(err, privateconnector.ErrUnsupported) {
		Fail(c, 409, "merge_unsupported", "Required native protection or queue evidence is unsupported; inspect the provider configuration", false)
		return
	}
	IdentityFailure(c, err)
}
