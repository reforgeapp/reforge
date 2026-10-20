package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/agent"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
)

type qualificationRequest struct {
	EvidenceID            string `json:"evidence_id"`
	CheckedAt             string `json:"checked_at"`
	ExpiresAt             string `json:"expires_at"`
	AuthCustody           bool   `json:"auth_custody"`
	NativeToolContainment bool   `json:"native_tool_containment"`
	Terms                 bool   `json:"terms"`
	Topology              bool   `json:"topology"`
	Entitlement           bool   `json:"entitlement"`
	Quota                 bool   `json:"quota"`
	NoPaidOverage         bool   `json:"no_paid_overage"`
}

func (s *Server) RegisterAgentQualification(qualifications *agent.QualificationService, connectionsService *connections.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID/connections/:connectionID", s.IdentitySession())
	group.GET("/agent-qualification", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		connection, err := connectionsService.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		qualification, found, err := qualifications.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		response := gin.H{"capabilities": agent.Capabilities(qualification, binding(connection), connection.Provider, found)}
		if found {
			response["qualification"] = qualification
		}
		c.JSON(http.StatusOK, response)
	})
	group.PUT("/agent-qualification", func(c *gin.Context) {
		var in qualificationRequest
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		connection, err := connectionsService.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"))
		if err != nil {
			connectionFailure(c, err)
			return
		}
		if connection.Kind != "agent" {
			Fail(c, http.StatusBadRequest, "invalid_request", "Qualification applies to agent connections", false)
			return
		}
		checked, err := time.Parse(time.RFC3339, in.CheckedAt)
		if err != nil {
			Fail(c, http.StatusBadRequest, "invalid_request", "checked_at must be RFC3339", false)
			return
		}
		expires, err := time.Parse(time.RFC3339, in.ExpiresAt)
		if err != nil {
			Fail(c, http.StatusBadRequest, "invalid_request", "expires_at must be RFC3339", false)
			return
		}
		qualification := agent.Qualification{
			Binding: binding(connection), EvidenceID: in.EvidenceID, CheckedAt: checked, ExpiresAt: expires,
			AuthCustody: in.AuthCustody, NativeToolContainment: in.NativeToolContainment, Terms: in.Terms,
			Topology: in.Topology, Entitlement: in.Entitlement, Quota: in.Quota, NoPaidOverage: in.NoPaidOverage,
		}
		stored, err := qualifications.Put(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID"), qualification)
		if err != nil {
			agentQualificationFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"qualification": stored, "capabilities": agent.Capabilities(stored, stored.Binding, connection.Provider, true)})
	})
	group.DELETE("/agent-qualification", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := qualifications.Delete(c.Request.Context(), session, c.Param("orgID"), c.Param("connectionID")); err != nil {
			agentQualificationFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
}

func binding(connection connections.Connection) agent.Binding {
	runner := ""
	if connection.Route != nil {
		runner = connection.Route.RunnerID
	}
	return agent.Binding{
		OrgID:             connection.OrgID,
		ConnectionID:      connection.ID,
		RunnerID:          runner,
		ConnectionVersion: connection.Version,
		CredentialVersion: connection.CredentialVersion,
		AccountID:         connection.Settings.Namespace,
		Model:             connection.Settings.Model,
		RuntimeDigest:     connection.Settings.RuntimeVersion,
		Deployment:        connection.Settings.Profile,
	}
}

func agentQualificationFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent.ErrInvalidQualification):
		Fail(c, http.StatusBadRequest, "invalid_qualification", "Qualification evidence is incomplete or expired", false)
	case errors.Is(err, auth.ErrConflict):
		Fail(c, http.StatusConflict, "agent_qualification_conflict", "Connection or qualification changed; refresh before retrying", false)
	default:
		IdentityFailure(c, err)
	}
}
