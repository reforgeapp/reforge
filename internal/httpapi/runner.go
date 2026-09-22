package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/runner"
	"reforge/internal/workflow"
)

func (s *Server) RegisterRunner(service *runner.Service) {
	browser := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	browser.GET("/runner-pools", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		filter := runner.PoolFilter{Query: strings.TrimSpace(c.Query("q")), State: strings.TrimSpace(c.Query("state"))}
		value, err := service.Pools(c.Request.Context(), session, c.Param("orgID"), filter, limit, cursor)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	browser.GET("/runner-pools/:poolID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Pool(c.Request.Context(), session, c.Param("orgID"), c.Param("poolID"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	browser.POST("/runner-pools", func(c *gin.Context) {
		var in runner.PoolInput
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.PutPool(c.Request.Context(), session, c.Param("orgID"), "", in, 0, c.GetString("request_id"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
		c.JSON(201, value)
	})
	browser.PUT("/runner-pools/:poolID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var in runner.PoolInput
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.PutPool(c.Request.Context(), session, c.Param("orgID"), c.Param("poolID"), in, version, c.GetString("request_id"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
		c.JSON(200, value)
	})
	browser.POST("/runner-pools/:poolID/enrollments", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.EnrollToken(c.Request.Context(), session, c.Param("orgID"), c.Param("poolID"), c.GetString("request_id"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(201, gin.H{"token": value.Token, "expires_at": value.ExpiresAt})
	})
	browser.GET("/runner-pools/:poolID/runners", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Runners(c.Request.Context(), session, c.Param("orgID"), c.Param("poolID"), limit, cursor)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	browser.DELETE("/runners/:runnerID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		if err := service.Revoke(c.Request.Context(), session, c.Param("orgID"), c.Param("runnerID"), version, c.GetString("request_id")); err != nil {
			runnerFailure(c, err)
			return
		}
		c.Status(204)
	})
	browser.GET("/artifacts/:artifactID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Metadata(c.Request.Context(), session, c.Param("orgID"), c.Param("artifactID"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	browser.GET("/artifacts/:artifactID/download", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		m, r, err := service.Download(c.Request.Context(), session, c.Param("orgID"), c.Param("artifactID"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		defer r.Close()
		sendArtifact(c, m, r)
	})
	workers := s.Router.Group("/runner/v1", func(c *gin.Context) {
		origin, err := url.Parse(s.Config.PublicURL)
		if err != nil || c.Request.Host != origin.Host || c.GetHeader("Origin") != "" || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			IdentityFailure(c, auth.ErrForbidden)
			return
		}
		value := c.GetHeader("Authorization")
		if !strings.HasPrefix(value, "Bearer ") || len(value) > 256 {
			IdentityFailure(c, auth.ErrUnauthenticated)
			return
		}
		c.Set("runner_token", strings.TrimPrefix(value, "Bearer "))
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	workers.POST("/enroll", func(c *gin.Context) {
		var in struct {
			Name string `json:"name"`
		}
		if !identityJSON(c, &in) {
			return
		}
		value, err := service.Enroll(c.Request.Context(), c.GetString("runner_token"), in.Name)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(201, gin.H{"token": value.Token, "expires_at": value.ExpiresAt, "runner": value.Runner})
	})
	workers.POST("/rotate", func(c *gin.Context) {
		value, err := service.Rotate(c.Request.Context(), c.GetString("runner_token"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"token": value.Token, "expires_at": value.ExpiresAt, "runner": value.Runner})
	})
	workers.POST("/claim", func(c *gin.Context) {
		value, err := service.Claim(c.Request.Context(), c.GetString("runner_token"))
		if errors.Is(err, workflow.ErrNoWork) {
			c.Status(204)
			return
		}
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"token": value.Credential.Token, "expires_at": value.Credential.ExpiresAt, "lease": value.Lease, "task": value.Task})
	})
	workers.POST("/heartbeat", func(c *gin.Context) {
		value, err := service.Heartbeat(c.Request.Context(), c.GetString("runner_token"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	workers.POST("/progress", func(c *gin.Context) {
		var in struct {
			State domain.TaskState `json:"state"`
		}
		if !identityJSON(c, &in) {
			return
		}
		value, err := service.Progress(c.Request.Context(), c.GetString("runner_token"), in.State)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	workers.POST("/result", func(c *gin.Context) {
		var in workflow.Completion
		if !identityJSON(c, &in) {
			return
		}
		value, err := service.Complete(c.Request.Context(), c.GetString("runner_token"), in)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	workers.POST("/artifacts", func(c *gin.Context) {
		_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(10 * time.Second))
		media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil {
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, artifact.MaxSize+1)
		value, err := service.Upload(c.Request.Context(), c.GetString("runner_token"), c.GetHeader("X-Artifact-Name"), media, c.Request.Body)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.JSON(201, value)
	})
	workers.GET("/artifacts/:artifactID", func(c *gin.Context) {
		m, r, err := service.DownloadJob(c.Request.Context(), c.GetString("runner_token"), c.Param("artifactID"))
		if err != nil {
			runnerFailure(c, err)
			return
		}
		defer r.Close()
		sendArtifact(c, m, r)
	})
	workers.POST("/operations/:operation", func(c *gin.Context) {
		var in struct {
			ConnectionID string          `json:"connection_id"`
			Input        json.RawMessage `json:"input"`
		}
		if !identityJSON(c, &in) {
			return
		}
		value, err := service.Broker(c.Request.Context(), c.GetString("runner_token"), c.Param("operation"), in.ConnectionID, in.Input)
		if err != nil {
			runnerFailure(c, err)
			return
		}
		c.Data(200, "application/json", value)
	})
}
func sendArtifact(c *gin.Context, m artifact.Metadata, r io.Reader) {
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": m.Name}))
	c.Header("Content-Length", strconv.FormatInt(m.Size, 10))
	c.Header("Cache-Control", "no-store")
	c.Status(200)
	_, _ = io.Copy(c.Writer, r)
}
func runnerFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runner.ErrUnsupported):
		Fail(c, 409, "operation_unavailable", "Runner operation is unavailable", false)
	case errors.Is(err, artifact.ErrContent):
		Fail(c, 400, "artifact_rejected", "Artifact content is not permitted", false)
	case errors.Is(err, artifact.ErrQuota):
		Fail(c, 413, "artifact_limit", "Artifact storage limit reached", false)
	default:
		workflowFailure(c, err)
	}
}
