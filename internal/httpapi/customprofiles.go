package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/customcmd"
)

func (s *Server) RegisterCustomProfiles(service *customcmd.Service) {
	group := s.Router.Group("/api/v1/orgs/:orgID", s.IdentitySession())
	group.GET("/custom-profiles", func(c *gin.Context) {
		limit, cursor, ok := identityPage(c)
		if !ok {
			return
		}
		if limit > 100 {
			Fail(c, http.StatusBadRequest, "invalid_request", "Profile page size must be between 1 and 100", false)
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.List(c.Request.Context(), session, c.Param("orgID"), cursor, limit)
		if err != nil {
			customProfileFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	group.POST("/custom-profiles", func(c *gin.Context) {
		var in customcmd.Profile
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Create(c.Request.Context(), session, c.Param("orgID"), in, c.GetString("request_id"))
		if err != nil {
			customProfileFailure(c, err)
			return
		}
		customProfileResponse(c, http.StatusCreated, out)
	})
	group.GET("/custom-profiles/:profileID", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		out, err := service.Get(c.Request.Context(), session, c.Param("orgID"), c.Param("profileID"))
		if err != nil {
			customProfileFailure(c, err)
			return
		}
		customProfileResponse(c, http.StatusOK, out)
	})
	group.POST("/custom-profiles/:profileID/approve", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		var in struct {
			Evidence string `json:"evidence"`
		}
		if !identityJSON(c, &in) {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Approve(c.Request.Context(), session, c.Param("orgID"), c.Param("profileID"), in.Evidence, version, c.GetString("request_id"))
		if err != nil {
			customProfileFailure(c, err)
			return
		}
		customProfileResponse(c, http.StatusOK, out)
	})
	group.POST("/custom-profiles/:profileID/revoke", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		out, err := service.Revoke(c.Request.Context(), session, c.Param("orgID"), c.Param("profileID"), version, c.GetString("request_id"))
		if err != nil {
			customProfileFailure(c, err)
			return
		}
		customProfileResponse(c, http.StatusOK, out)
	})
}

func customProfileResponse(c *gin.Context, status int, value customcmd.Profile) {
	c.Header("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	c.JSON(status, value)
}

func customProfileFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, customcmd.ErrNotApproved):
		Fail(c, http.StatusConflict, "custom_profile_unavailable", "Profile is not approved or no longer exists", false)
	case errors.Is(err, customcmd.ErrInvalid):
		Fail(c, http.StatusBadRequest, "invalid_request", "Profile definition is invalid", false)
	case errors.Is(err, auth.ErrConflict):
		Fail(c, http.StatusConflict, "custom_profile_conflict", "Profile version changed; refresh and review before retrying", false)
	default:
		IdentityFailure(c, err)
	}
}
