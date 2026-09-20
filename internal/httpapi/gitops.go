package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/gitops"
	"reforge/internal/privateconnector"
)

func (s *Server) RegisterGitOps(service *gitops.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.GET("/gitops-configurations", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Configurations(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	})
	group.PUT("/gitops-configurations/:environment", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var in gitops.Configuration
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
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusOK, out.Version, out)
	})
	group.POST("/gitops-configurations/:environment/preview", func(c *gin.Context) {
		var in gitops.PreviewRequest
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Preview(c.Request.Context(), session, c.Param("orgID"), c.Param("environment"), in, c.GetString("request_id"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusOK, out.Configuration.Version, out)
	})
	group.GET("/gitops-promotions", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.List(c.Request.Context(), session, c.Param("orgID"), c.Query("environment"), cursor, limit)
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	group.POST("/gitops-promotions", func(c *gin.Context) {
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
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusCreated, out.Version, out)
	})
	group.GET("/gitops-promotions/:promotionID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("promotionID"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusOK, out.Promotion.Version, out)
	})
	group.POST("/gitops-promotions/:promotionID/continue", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Continue(c.Request.Context(), session, c.Param("orgID"), c.Param("promotionID"), c.GetString("request_id"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusOK, out.Version, out)
	})
	group.POST("/gitops-promotions/:promotionID/observe", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.ObserveAs(c.Request.Context(), session, c.Param("orgID"), c.Param("promotionID"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusOK, out.Version, out)
	})
	group.POST("/gitops-promotions/:promotionID/cancel", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Cancel(c.Request.Context(), session, c.Param("orgID"), c.Param("promotionID"), version, c.GetString("request_id"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusOK, out.Version, out)
	})
	group.POST("/gitops-promotions/:promotionID/merge-preview", func(c *gin.Context) {
		var in struct {
			Method string `json:"method"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.MergePreview(c.Request.Context(), session, c.Param("orgID"), c.Param("promotionID"), in.Method, c.GetString("request_id"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	group.POST("/gitops-promotions/:promotionID/merge", func(c *gin.Context) {
		var in struct {
			GateID         string `json:"gate_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.RequestMerge(c.Request.Context(), session, c.Param("orgID"), c.Param("promotionID"), in.GateID, in.IdempotencyKey, c.GetString("request_id"))
		if err != nil {
			gitopsFailure(c, err)
			return
		}
		versioned(c, http.StatusCreated, out.Version, out)
	})
	s.Router.POST("/api/v1/gitops-health/:orgID/:promotionID", func(c *gin.Context) {
		var in gitops.HealthReport
		if !identityJSON(c, &in) {
			return
		}
		if err := service.ReceiveHealth(c.Request.Context(), c.Param("orgID"), c.Param("promotionID"), in, c.GetHeader("X-Reforge-Signature")); err != nil {
			gitopsFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
}

func versioned(c *gin.Context, status int, version int64, value any) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(version, 10)))
	c.JSON(status, value)
}

func gitopsFailure(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		Fail(c, 404, "not_found", "GitOps record not found", false)
		return
	}
	var provider *domain.ProviderError
	if errors.As(err, &provider) {
		if provider.Kind == "gitops_manifest_unsupported" {
			Fail(c, 409, "gitops_manifest_unsupported", provider.Message, false)
			return
		}
		Fail(c, 409, "gitops_native_unknown", "Native GitOps evidence is unavailable or unqualified; inspect the configured repositories and reconcile existing promotion", false)
		return
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		Fail(c, 409, "gitops_promotion_busy", "A promotion already owns this request; reconcile it before retrying", false)
		return
	}
	if errors.Is(err, privateconnector.ErrUnavailable) || errors.Is(err, privateconnector.ErrUnsupported) {
		Fail(c, 409, "gitops_transport_unavailable", "Reconnect the enrolled connector and verify GitOps adapter support", false)
		return
	}
	if errors.Is(err, auth.ErrConflict) {
		Fail(c, 409, "gitops_conflict", "GitOps evidence or version changed; refresh and reconcile before retrying", false)
		return
	}
	IdentityFailure(c, err)
}
