package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/agent"
	"github.com/reforgeapp/reforge/pkg/agentcontrol"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Server) RegisterNativeAgents(service *agentcontrol.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID/agents", s.IdentitySession())
	group.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Options(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			nativeAgentFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	group.POST("", func(c *gin.Context) {
		var in struct {
			Provider string `json:"provider"`
			Name     string `json:"name"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Create(c.Request.Context(), session, c.Param("orgID"), in.Provider, in.Name)
		if err != nil {
			nativeAgentFailure(c, err)
			return
		}
		c.JSON(http.StatusCreated, out)
	})
	group.GET("/:connectionID", func(c *gin.Context) {
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5 * time.Minute))
		session, _ := SessionFromContext(c)
		out, err := service.Action(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), agent.NativeAction{Kind: "status"})
		if err != nil {
			nativeAgentFailure(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, out)
	})
	group.POST("/:connectionID/default", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.UseDefault(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID")); err != nil {
			nativeAgentFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	group.POST("/:connectionID/:action", func(c *gin.Context) {
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5 * time.Minute))
		var in struct {
			LoginID string `json:"login_id"`
			Code    string `json:"code"`
			Model   string `json:"model"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Action(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), agent.NativeAction{Kind: c.Param("action"), LoginID: in.LoginID, Code: in.Code, Model: in.Model})
		if err != nil {
			nativeAgentFailure(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, out)
	})
}

func nativeAgentFailure(c *gin.Context, err error) {
	var provider *domain.ProviderError
	switch {
	case errors.As(err, &provider) && (provider.Kind == "quota" || provider.Kind == "rate_limit"):
		Fail(c, http.StatusTooManyRequests, "agent_allowance", "Subscription allowance or agent capacity unavailable; try again shortly", false)
	case errors.As(err, &provider) && provider.Kind == "auth":
		Fail(c, http.StatusConflict, "agent_sign_in", "Sign in to your agent account again", false)
	case errors.Is(err, agent.ErrDenied):
		Fail(c, http.StatusForbidden, "agent_denied", "Agent account does not match this connection; disconnect and sign in again", false)
	case errors.Is(err, privateconnector.ErrUnavailable):
		Fail(c, http.StatusServiceUnavailable, "agent_unavailable", "Agent runner unavailable; try again shortly", false)
	case errors.Is(err, agent.ErrDisabled):
		Fail(c, http.StatusConflict, "agent_unavailable", "Agent runtime unavailable; check runner connection", false)
	case errors.Is(err, agent.ErrProtocol), errors.Is(err, agent.ErrUncertain):
		Fail(c, http.StatusServiceUnavailable, "agent_unavailable", "Agent did not complete this request; try again", false)
	default:
		connectionFailure(c, err)
	}
}
