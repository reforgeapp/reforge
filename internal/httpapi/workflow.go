package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"reforge/internal/auth"
	"reforge/internal/workflow"
	"strconv"
	"time"
)

func (s *Server) RegisterWorkflow(service *workflow.Service) {
	g := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	g.GET("/tasks", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.List(c.Request.Context(), session, c.Param("orgID"), c.Query("state"), limit, cursor)
		if err != nil {
			workflowFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.POST("/tasks", func(c *gin.Context) {
		var input workflow.EnqueueInput
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Enqueue(c.Request.Context(), session, c.Param("orgID"), input, c.GetString("request_id"))
		if err != nil {
			workflowFailure(c, err)
			return
		}
		workflowResponse(c, 201, value)
	})
	g.GET("/tasks/:taskID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("taskID"))
		if err != nil {
			workflowFailure(c, err)
			return
		}
		workflowResponse(c, 200, value)
	})
	for _, action := range []string{"cancel", "resume"} {
		g.POST("/tasks/:taskID/"+action, func(c *gin.Context) {
			version, ok := identityVersion(c)
			if !ok {
				return
			}
			session, _ := SessionFromContext(c)
			var value workflow.Task
			var err error
			if action == "cancel" {
				value, err = service.Cancel(c.Request.Context(), session, c.Param("orgID"), c.Param("taskID"), version, c.GetString("request_id"))
			} else {
				value, err = service.Resume(c.Request.Context(), session, c.Param("orgID"), c.Param("taskID"), version, c.GetString("request_id"))
			}
			if err != nil {
				workflowFailure(c, err)
				return
			}
			workflowResponse(c, 200, value)
		})
	}
	g.PUT("/pauses/:scopeKind/:scopeID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input struct {
			Paused bool `json:"paused"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.SetPause(c.Request.Context(), session, c.Param("orgID"), workflow.Pause{Kind: c.Param("scopeKind"), ID: c.Param("scopeID"), Paused: input.Paused}, version, c.GetString("request_id"))
		if err != nil {
			workflowFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
		c.JSON(200, value)
	})
	g.GET("/events/replay", func(c *gin.Context) {
		after, ok := eventCursor(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		page, err := service.Replay(c.Request.Context(), session, c.Param("orgID"), after, 100)
		if err != nil {
			workflowFailure(c, err)
			return
		}
		c.JSON(200, page)
	})
	g.GET("/events", func(c *gin.Context) { s.workflowStream(c, service) })
}
func workflowResponse(c *gin.Context, status int, value workflow.Task) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	c.JSON(status, value)
}
func eventCursor(c *gin.Context) (int64, bool) {
	raw := c.GetHeader("Last-Event-ID")
	if raw == "" {
		raw = c.DefaultQuery("after", "0")
	}
	after, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || after < 0 {
		IdentityFailure(c, auth.ErrInvalid)
		return 0, false
	}
	return after, true
}
func (s *Server) workflowStream(c *gin.Context, service *workflow.Service) {
	after, ok := eventCursor(c)
	if !ok {
		return
	}
	session, _ := SessionFromContext(c)
	page, err := service.Replay(c.Request.Context(), session, c.Param("orgID"), after, 100)
	if err != nil {
		workflowFailure(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-store")
	c.Status(200)
	controller := http.NewResponseController(c.Writer)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	heartbeat := time.Now()
	for {
		_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err != nil {
			code := "stream_unavailable"
			if errors.Is(err, auth.ErrUnauthenticated) || errors.Is(err, auth.ErrForbidden) {
				code = "access_revoked"
			}
			if errors.Is(err, workflow.ErrCursor) {
				code = "cursor_expired"
			}
			_, _ = fmt.Fprintf(c.Writer, "event: reset\ndata: {\"code\":%q}\n\n", code)
			c.Writer.Flush()
			return
		}
		for _, event := range page.Items {
			body, encodeErr := json.Marshal(event)
			if encodeErr != nil {
				return
			}
			if _, writeErr := fmt.Fprintf(c.Writer, "id: %d\nevent: reforge\ndata: %s\n\n", event.ID, body); writeErr != nil {
				return
			}
		}
		if len(page.Items) > 0 || time.Since(heartbeat) > 10*time.Second {
			if _, err = fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
			heartbeat = time.Now()
		} else if after == 0 {
			c.Writer.Flush()
		}
		after = page.ScanAfter
		if page.Complete {
			select {
			case <-c.Request.Context().Done():
				return
			case <-ticker.C:
			}
		}
		page, err = service.Replay(c.Request.Context(), session, c.Param("orgID"), after, 100)
	}
}
func workflowFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workflow.ErrCursor):
		Fail(c, 410, "cursor_expired", "Event history expired; reload current state", false)
	case errors.Is(err, workflow.ErrFence):
		Fail(c, 409, "lease_expired", "Execution lease is no longer valid", false)
	case errors.Is(err, workflow.ErrPaused):
		Fail(c, 409, "automation_paused", "An applicable scope is paused", false)
	case errors.Is(err, workflow.ErrPolicy):
		Fail(c, 409, "policy_blocked", "Current policy does not permit this work; review its effective rules", false)
	case errors.Is(err, workflow.ErrReconciliation):
		Fail(c, 409, "reconciliation_required", "Previous external outcome must be reconciled before retry", false)
	default:
		IdentityFailure(c, err)
	}
}
