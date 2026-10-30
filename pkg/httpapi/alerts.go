package httpapi

import (
	"errors"
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/reforgeapp/reforge/pkg/alerts"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Server) RegisterAlerts(service *alerts.Service) {
	invites := s.Router.Group("/api/v1/orgs/:orgID/member-invitations", s.IdentitySession())
	invites.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		items, err := s.Auth.MemberInvitations(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"items": items})
	})
	invites.POST("", func(c *gin.Context) {
		var input struct {
			Email string      `json:"email"`
			Role  domain.Role `json:"role"`
		}
		if !identityJSON(c, &input) {
			return
		}
		if !slices.Contains([]domain.Role{domain.Owner, domain.Admin, domain.Maintainer, domain.Reviewer, domain.Viewer}, input.Role) {
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		session, _ := SessionFromContext(c)
		token, orgName, err := s.Auth.CreateMemberInvitation(c.Request.Context(), session, c.Param("orgID"), input.Email, input.Role, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		sent := service.Invite(c.Request.Context(), session, c.Param("orgID"), input.Email, orgName, token) == nil
		c.JSON(201, gin.H{"email_sent": sent, "link": s.Config.PublicURL + "/auth/join?token=" + token})
	})
	invites.DELETE("/:invitationID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := s.Auth.RevokeMemberInvitation(c.Request.Context(), session, c.Param("orgID"), c.Param("invitationID"), c.GetString("request_id")); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Status(204)
	})
	g := s.Router.Group("/api/v1/orgs/:orgID/alerts", s.IdentitySession())
	g.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		value, err := service.Get(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.PUT("", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input alerts.Settings
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		value, err := service.Put(c.Request.Context(), session, c.Param("orgID"), input, version)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, value)
	})
	g.POST("/test", func(c *gin.Context) {
		var input alerts.Settings
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		err := service.Test(c.Request.Context(), session, c.Param("orgID"), input)
		switch {
		case errors.Is(err, alerts.ErrNotConfigured):
			Fail(c, 409, "email_unconfigured", "Enter a mail server host and from address", false)
		case errors.Is(err, auth.ErrForbidden), errors.Is(err, auth.ErrInvalid):
			IdentityFailure(c, err)
		case err != nil:
			Fail(c, 502, "email_failed", "The mail server rejected the message: "+err.Error(), false)
		default:
			c.Status(204)
		}
	})
}
