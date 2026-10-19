package httpapi

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/internal/auth"
)

func (s *Server) RegisterOrgInvitations(service *auth.Service) {
	org := s.Router.Group("/api/v1/orgs/:orgID/identity/oidc/invitations", s.IdentitySession())
	org.GET("", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		invitations, err := service.OrgOIDCInvitations(c.Request.Context(), session, c.Param("orgID"), limit, cursor)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, invitations)
	})
	org.POST("", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
		var input auth.OrgOIDCInvitationInput
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		invitation, err := service.CreateOrgOIDCInvitation(c.Request.Context(), session, c.Param("orgID"), input, c.GetString("request_id"))
		if err != nil {
			oidcFailure(c, err)
			return
		}
		c.JSON(http.StatusCreated, invitation)
	})
	org.DELETE("/:invitationID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.RevokeOrgOIDCInvitation(c.Request.Context(), session, c.Param("orgID"), c.Param("invitationID"), c.GetString("request_id")); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	s.Router.POST("/auth/invitations/redeem", func(c *gin.Context) {
		c.Header("Referrer-Policy", "no-referrer")
		if err := service.CheckInvitationRequest(c.Request); err != nil {
			IdentityFailure(c, err)
			return
		}
		media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || media != "application/x-www-form-urlencoded" || c.Request.URL.RawQuery != "" {
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
		if err := c.Request.ParseForm(); err != nil {
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		values := c.Request.PostForm["token"]
		if len(values) != 1 || len(values[0]) != 43 {
			IdentityFailure(c, auth.ErrUnauthenticated)
			return
		}
		location, err := service.BeginOrgOIDCInvitation(c.Request.Context(), c.Writer, values[0])
		if err != nil {
			loginFailure(c, true, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"authorization_url": location})
	})
}
