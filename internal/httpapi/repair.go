package httpapi

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/maintenance/recipes"
	"reforge/internal/maintenance/repair"
	"reforge/internal/workflow"
	"strconv"
	"strings"
	"time"
)

func (s *Server) RegisterRepair(service *repair.Service) {
	browser := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	browser.GET("/repositories/:repoID/repair-baseline", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Baseline(c.Request.Context(), session, c.Param("orgID"), c.Param("repoID"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"run": out})
	})
	browser.GET("/repair-recipes", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if _, err := s.Auth.ResolveActor(c.Request.Context(), session, c.Param("orgID")); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, service.Recipes())
	})
	for _, action := range []string{"repair-preview", "repair-runs"} {
		browser.POST("/"+action, func(c *gin.Context) {
			var in repair.Input
			if !identityJSON(c, &in) {
				return
			}
			session, _ := SessionFromContext(c)
			_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
			if action == "repair-preview" {
				out, err := service.Preview(c.Request.Context(), session, c.Param("orgID"), in)
				if err != nil {
					repairFailure(c, err)
					return
				}
				c.JSON(200, out)
			} else {
				out, err := service.Enqueue(c.Request.Context(), session, c.Param("orgID"), in, c.GetString("request_id"))
				if err != nil {
					repairFailure(c, err)
					return
				}
				c.JSON(201, out)
			}
		})
	}
	browser.GET("/repair-runs/:taskID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("taskID"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	browser.GET("/repair-runs/:taskID/logs", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
		out, err := service.Logs(c.Request.Context(), session, c.Param("orgID"), c.Param("taskID"), max(after, 0))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	browser.POST("/repair-runs/:taskID/reconcile", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Reconcile(c.Request.Context(), session, c.Param("orgID"), c.Param("taskID"), version, c.GetString("request_id"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs := s.Router.Group("/runner/v1/repair", func(c *gin.Context) {
		origin, err := url.Parse(s.Config.PublicURL)
		token := c.GetHeader("Authorization")
		if err != nil || c.Request.Host != origin.Host || c.GetHeader("Origin") != "" || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			IdentityFailure(c, auth.ErrForbidden)
			return
		}
		if !strings.HasPrefix(token, "Bearer ") || len(token) > 263 {
			IdentityFailure(c, auth.ErrUnauthenticated)
			return
		}
		c.Set("runner_token", strings.TrimPrefix(token, "Bearer "))
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	jobs.GET("/run", func(c *gin.Context) {
		out, err := service.JobRun(c.Request.Context(), c.GetString("runner_token"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs.GET("/context", func(c *gin.Context) {
		out, err := service.Context(c.Request.Context(), c.GetString("runner_token"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs.GET("/source/:sha", func(c *gin.Context) {
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		out, err := service.Snapshot(c.Request.Context(), c.GetString("runner_token"), c.Param("sha"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs.POST("/stage", func(c *gin.Context) {
		out, err := service.Stage(c.Request.Context(), c.GetString("runner_token"))
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs.POST("/publish", func(c *gin.Context) {
		var in repair.Publication
		if !identityJSON(c, &in) {
			return
		}
		out, err := service.Publish(c.Request.Context(), c.GetString("runner_token"), in)
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs.POST("/native-checks", func(c *gin.Context) {
		var in repair.Publication
		if !identityJSON(c, &in) {
			return
		}
		out, err := service.RecordNative(c.Request.Context(), c.GetString("runner_token"), in)
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
	jobs.POST("/logs", func(c *gin.Context) {
		var in []repair.LogEntry
		if !identityJSON(c, &in) {
			return
		}
		if err := service.AppendLogs(c.Request.Context(), c.GetString("runner_token"), in); err != nil {
			repairFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	jobs.POST("/checkpoint", func(c *gin.Context) {
		var in repair.Checkpoint
		if !identityJSON(c, &in) {
			return
		}
		if err := service.SaveCheckpoint(c.Request.Context(), c.GetString("runner_token"), in); err != nil {
			repairFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	jobs.POST("/report", func(c *gin.Context) {
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
		var in repair.Report
		if !identityJSON(c, &in) {
			return
		}
		out, err := service.SaveReport(c.Request.Context(), c.GetString("runner_token"), in)
		if err != nil {
			repairFailure(c, err)
			return
		}
		c.JSON(200, out)
	})
}
func repairFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repair.ErrRunNotFound):
		Fail(c, 404, "repair_not_found", "This task has no repair execution record", false)
	case errors.Is(err, budget.ErrCapacity), errors.Is(err, budget.ErrUnknown), errors.Is(err, budget.ErrConflict), errors.Is(err, budget.ErrRevoked), errors.Is(err, budget.ErrInvalid):
		budgetFailure(c, err)
	case errors.Is(err, workflow.ErrPolicy), errors.Is(err, workflow.ErrReconciliation), errors.Is(err, workflow.ErrPaused), errors.Is(err, workflow.ErrFence):
		workflowFailure(c, err)
	case errors.Is(err, repair.ErrValidation), errors.Is(err, repair.ErrPatch):
		Fail(c, 409, "validation_rejected", "Frozen validation or patch evidence is incomplete; review run artifacts", false)
	case errors.Is(err, repair.ErrSourceMoved):
		Fail(c, 409, "source_moved", "The native source or target moved; refresh discovery before publication", false)
	case errors.Is(err, recipes.ErrUnsupported):
		Fail(c, 409, "recipe_unsupported", "This recipe needs supported tests and preinstalled dependencies in a verified runner image", false)
	default:
		discoveryFailure(c, err)
	}
}
