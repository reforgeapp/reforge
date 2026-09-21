package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"reforge/internal/agent"
	"reforge/internal/auth"
)

func (s *Server) RegisterAgentAuth(service *agent.AuthService) {
	group := s.Router.Group("/api/v1/orgs/:orgID/connections/:connectionID", s.IdentitySession())
	group.POST("/agent/login", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		login, err := service.Login(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"))
		if err != nil {
			agentAuthFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"login": login})
	})
	group.POST("/agent/logout", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.Logout(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID")); err != nil {
			agentAuthFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
}

func agentAuthFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent.ErrDisabled):
		Fail(c, http.StatusConflict, "agent_runtime_disabled", "The official runtime is not configured, isolated or qualified for this connection", false)
	case errors.Is(err, agent.ErrDenied):
		Fail(c, http.StatusForbidden, "agent_action_denied", "The official runtime denied this action", false)
	case errors.Is(err, agent.ErrUncertain):
		Fail(c, http.StatusConflict, "agent_uncertain", "The official runtime outcome is uncertain; reconcile before retrying", false)
	case errors.Is(err, auth.ErrForbidden):
		Fail(c, http.StatusForbidden, "forbidden", "Owner or administrator access is required", false)
	default:
		connectionFailure(c, err)
	}
}
