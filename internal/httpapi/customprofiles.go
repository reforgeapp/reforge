package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/customcmd"
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

func (s *Server) RegisterCustomDispatch(dispatcher *customcmd.Dispatcher) {
	jobs := s.Router.Group("/runner/v1/repair/custom", func(c *gin.Context) {
		origin, err := url.Parse(s.Config.PublicURL)
		token := c.GetHeader("Authorization")
		if dispatcher == nil || err != nil || c.Request.Host != origin.Host || c.GetHeader("Origin") != "" || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
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
	jobs.POST("/authorize", func(c *gin.Context) {
		out, err := dispatcher.Authorize(c.Request.Context(), c.GetString("runner_token"))
		if err != nil {
			customDispatchFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
	jobs.POST("/report", func(c *gin.Context) {
		var in customcmd.ReportInput
		if !identityJSON(c, &in) {
			return
		}
		out, err := dispatcher.Report(c.Request.Context(), c.GetString("runner_token"), in)
		if err != nil {
			customDispatchFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	})
}

func customDispatchFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, customcmd.ErrNoProfile):
		Fail(c, http.StatusConflict, "custom_profile_unbound", "This task is not bound to a custom command profile", false)
	case errors.Is(err, customcmd.ErrNotApproved):
		Fail(c, http.StatusConflict, "custom_profile_unavailable", "Profile is not approved, revoked or changed; refresh the run", false)
	case errors.Is(err, customcmd.ErrCapacity):
		Fail(c, http.StatusConflict, "custom_profile_capacity", "Profile concurrency limit reached; retry after an active run finishes", true)
	case errors.Is(err, auth.ErrConflict):
		Fail(c, http.StatusConflict, "custom_profile_conflict", "Run state changed; reload before retrying", false)
	default:
		repairFailure(c, err)
	}
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
