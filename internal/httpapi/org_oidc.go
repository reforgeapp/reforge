package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"reforge/internal/auth"
)

func (s *Server) RegisterOrgOIDC() {
	group := s.Router.Group("/api/v1/orgs/:orgID/identity/oidc", s.IdentitySession())
	group.GET("", func(c *gin.Context) {
		session, _ := SessionFromContext(c)
		settings, err := s.Auth.OrgOIDC(c.Request.Context(), session, c.Param("orgID"))
		if err != nil {
			IdentityFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, settings)
	})
	group.PUT("", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 70<<10)
		var input auth.OrgOIDCInput
		if !identityJSON(c, &input) {
			input.ClientSecret = ""
			return
		}
		session, _ := SessionFromContext(c)
		settings, err := s.Auth.PutOrgOIDC(c.Request.Context(), session, c.Param("orgID"), input, version, c.GetString("request_id"))
		input.ClientSecret = ""
		if err != nil {
			oidcFailure(c, err)
			return
		}
		c.Header("ETag", `"`+strconv.FormatInt(settings.Version, 10)+`"`)
		c.JSON(http.StatusOK, settings)
	})
	group.POST("/probe", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		settings, err := s.Auth.ProbeOrgOIDC(c.Request.Context(), session, c.Param("orgID"), version, c.GetString("request_id"))
		if err != nil {
			oidcFailure(c, err)
			return
		}
		c.JSON(http.StatusOK, settings)
	})
	group.POST("/activate", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		if err := s.Auth.ActivateOrgOIDC(c.Request.Context(), session, c.Param("orgID"), version, c.GetString("request_id")); err != nil {
			oidcFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	group.POST("/disable", func(c *gin.Context) {
		version, ok := identityVersion(c)
		if !ok {
			return
		}
		session, _ := SessionFromContext(c)
		settings, err := s.Auth.DisableOrgOIDC(c.Request.Context(), session, c.Param("orgID"), version, c.GetString("request_id"))
		if err != nil {
			oidcFailure(c, err)
			return
		}
		c.Header("ETag", `"`+strconv.FormatInt(settings.Version, 10)+`"`)
		c.JSON(http.StatusOK, settings)
	})
}

func oidcFailure(c *gin.Context, err error) {
	var cooldown *auth.ProbeCooldownError
	if errors.As(err, &cooldown) {
		c.Header("Retry-After", strconv.Itoa(cooldown.RetryAfter))
		Fail(c, http.StatusTooManyRequests, "probe_cooldown", "Wait before probing this issuer again", true)
		return
	}
	if errors.Is(err, auth.ErrOIDCProbeBusy) {
		c.Header("Retry-After", "1")
		Fail(c, http.StatusTooManyRequests, "probe_capacity", "Issuer probe capacity is busy", true)
		return
	}
	switch {
	case errors.Is(err, auth.ErrOIDCNotReady):
		Fail(c, http.StatusConflict, "activation_unavailable", "Organisation login is not available until org-aware login support is implemented", false)
	case errors.Is(err, auth.ErrOIDCProbe):
		Fail(c, http.StatusUnprocessableEntity, "issuer_unverified", "Issuer metadata could not be verified", false)
	case errors.Is(err, auth.ErrOIDCUnavailable):
		Fail(c, http.StatusServiceUnavailable, "secret_storage_unavailable", "Identity secret storage is unavailable", true)
	default:
		IdentityFailure(c, err)
	}
}
