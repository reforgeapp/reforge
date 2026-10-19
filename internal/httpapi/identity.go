package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/internal/auth"
)

func (s *Server) RegisterIdentity(service *auth.Service) {
	s.Auth = service
	s.RegisterOrgOIDC()
	s.RegisterOrgInvitations(service)
	s.Router.GET("/auth/login", func(c *gin.Context) {
		if err := service.CheckRequest(c.Request, ""); err != nil {
			IdentityFailure(c, err)
			return
		}
		orgIDs := c.Request.URL.Query()["org_id"]
		if len(orgIDs) > 1 || len(orgIDs) == 1 && orgIDs[0] == "" {
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		location, err := service.Login(c.Request.Context(), c.Writer, orgIDs...)
		if err != nil {
			loginFailure(c, len(orgIDs) > 0, err)
			return
		}
		c.Redirect(http.StatusFound, location)
	})
	s.Router.GET("/auth/callback", func(c *gin.Context) {
		if err := service.CheckRequest(c.Request, ""); err != nil {
			IdentityFailure(c, err)
			return
		}
		if err := service.Callback(c.Request.Context(), c.Writer, c.Request); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Redirect(http.StatusFound, "/")
	})
	s.Router.GET("/api/v1/session", s.IdentitySession(), func(c *gin.Context) { session, _ := SessionFromContext(c); c.JSON(200, session) })
	s.Router.POST("/auth/logout", s.IdentitySession(), func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.Logout(c.Request.Context(), session); err != nil {
			IdentityFailure(c, err)
			return
		}
		service.ClearSession(c.Writer)
		c.Status(204)
	})
	s.Router.DELETE("/api/v1/session", s.IdentitySession(), func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		if err := service.RevokeSessions(c.Request.Context(), session); err != nil {
			IdentityFailure(c, err)
			return
		}
		service.ClearSession(c.Writer)
		c.Status(204)
	})
	s.Router.POST("/auth/bootstrap", s.IdentitySession(), func(c *gin.Context) {
		var input struct {
			Token string `json:"token"`
			Name  string `json:"name"`
		}
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		org, err := service.Bootstrap(c.Request.Context(), session, input.Token, input.Name, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(201, org)
	})
	org := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	org.GET("/memberships", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		page, err := service.Members(c.Request.Context(), session, c.Param("orgID"), limit, cursor)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, page)
	})
	org.PUT("/memberships/:userID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input auth.Membership
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		result, err := service.PutMember(c.Request.Context(), session, c.Param("orgID"), c.Param("userID"), input, version, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(result.Version, 10)))
		c.JSON(200, result)
	})
	org.DELETE("/memberships/:userID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		if err := service.DeleteMember(c.Request.Context(), session, c.Param("orgID"), c.Param("userID"), version, c.GetString("request_id")); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Status(204)
	})
	org.GET("/teams", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		page, err := service.Teams(c.Request.Context(), session, c.Param("orgID"), limit, cursor)
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(200, page)
	})
	org.PUT("/teams/:teamID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var input auth.Team
		if !identityJSON(c, &input) {
			return
		}
		session, _ := SessionFromContext(c)
		result, err := service.PutTeam(c.Request.Context(), session, c.Param("orgID"), c.Param("teamID"), input, version, c.GetString("request_id"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Header("ETag", strconv.Quote(strconv.FormatInt(result.Version, 10)))
		c.JSON(200, result)
	})
	org.DELETE("/teams/:teamID", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		if err := service.DeleteTeam(c.Request.Context(), session, c.Param("orgID"), c.Param("teamID"), version, c.GetString("request_id")); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Status(204)
	})
}

func (s *Server) IdentitySession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := s.Auth.CheckRequest(&http.Request{Method: "GET", Host: c.Request.Host}, ""); err != nil {
			IdentityFailure(c, err)
			return
		}
		session, err := s.Auth.Authenticate(c.Request.Context(), s.Auth.RequestToken(c.Request))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		if err = s.Auth.CheckRequest(c.Request, session.CSRFToken); err != nil {
			IdentityFailure(c, err)
			return
		}
		c.Set("identity_session", session)
		c.Next()
	}
}
func SessionFromContext(c *gin.Context) (auth.Session, bool) {
	v, ok := c.Get("identity_session")
	if !ok {
		return auth.Session{}, false
	}
	session, ok := v.(auth.Session)
	return session, ok
}
func loginFailure(c *gin.Context, orgScoped bool, err error) {
	if orgScoped {
		oidcFailure(c, err)
		return
	}
	IdentityFailure(c, err)
}

func IdentityFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		Fail(c, 401, "unauthenticated", "Authentication required", false)
	case errors.Is(err, auth.ErrForbidden):
		Fail(c, 403, "forbidden", "Access denied", false)
	case errors.Is(err, auth.ErrConflict):
		Fail(c, 409, "conflict", "Version conflict or last owner", false)
	case errors.Is(err, auth.ErrInvalid):
		Fail(c, 400, "invalid_request", "Invalid request", false)
	default:
		Fail(c, 500, "internal", "Request failed", false)
	}
}
func identityJSON(c *gin.Context, value any) bool {
	if media := strings.Split(c.GetHeader("Content-Type"), ";")[0]; media != "application/json" {
		Fail(c, 415, "unsupported_media_type", "JSON required", false)
		return false
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		IdentityFailure(c, auth.ErrInvalid)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		IdentityFailure(c, auth.ErrInvalid)
		return false
	}
	return true
}
func identityVersion(c *gin.Context) (int64, bool) {
	raw := c.GetHeader("If-Match")
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		Fail(c, 428, "precondition_required", "If-Match version required", false)
		return 0, false
	}
	version, err := strconv.ParseInt(strings.Trim(raw, "\""), 10, 64)
	if err != nil || version < 0 {
		IdentityFailure(c, auth.ErrInvalid)
		return 0, false
	}
	return version, true
}
func identityPage(c *gin.Context) (int, string, bool) {
	limit := 50
	var err error
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	cursor := c.Query("cursor")
	if err != nil || limit < 1 || limit > 200 || (cursor != "" && !auth.ValidID(cursor)) {
		IdentityFailure(c, auth.ErrInvalid)
		return 0, "", false
	}
	return limit, cursor, true
}
