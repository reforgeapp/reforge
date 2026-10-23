package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/modelbroker"
)

func (s *Server) RegisterModelBroker(service *modelbroker.Service) {
	s.Router.POST("/runner/v1/model-turns", func(c *gin.Context) {
		origin, err := url.Parse(s.Config.PublicURL)
		token := c.GetHeader("Authorization")
		if service == nil || err != nil || c.Request.Host != origin.Host || c.GetHeader("Origin") != "" || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			IdentityFailure(c, auth.ErrForbidden)
			return
		}
		if !strings.HasPrefix(token, "Bearer ") || len(token) > 263 {
			IdentityFailure(c, auth.ErrUnauthenticated)
			return
		}
		if strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
			Fail(c, 415, "unsupported_media_type", "JSON required", false)
			return
		}
		c.Header("Cache-Control", "no-store")
		_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(5 * time.Second))
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, model.MaxRequestBytes/2))
		if err != nil {
			slog.WarnContext(c.Request.Context(), "model turn request rejected", "bytes", len(body), "error", err)
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		var in model.Turn
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || !in.Valid() {
			slog.WarnContext(c.Request.Context(), "model turn request rejected", "bytes", len(body), "messages", len(in.Messages), "continuation_bytes", len(in.Continuation))
			IdentityFailure(c, auth.ErrInvalid)
			return
		}
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(time.Duration(in.TimeoutMS)*time.Millisecond + 15*time.Second))
		result, err := service.Turn(c.Request.Context(), strings.TrimPrefix(token, "Bearer "), in)
		var provider *domain.ProviderError
		switch {
		case errors.As(err, &provider) && provider.Kind == "agent_auth":
			c.Header("Retry-After", "900")
			code := "agent_sign_in_required"
			if provider.Uncertain {
				code = "agent_sign_in_uncertain"
			}
			Fail(c, http.StatusUnauthorized, code, "Sign in to your agent account again; work resumes automatically", false)
		case errors.As(err, &provider) && provider.Kind == "billing":
			retryAfter := max(provider.RetryAfter, 15*time.Minute)
			c.Header("Retry-After", strconv.FormatInt(int64(retryAfter/time.Second), 10))
			code := "model_billing_unavailable"
			if provider.Uncertain {
				code = "model_billing_uncertain"
			}
			Fail(c, http.StatusPaymentRequired, code, "Model provider allowance unavailable; check subscription/billing", false)
		case errors.As(err, &provider) && (provider.Kind == "quota" || provider.Kind == "rate_limit" || provider.Kind == "rate_limited"):
			if provider.RetryAfter > 0 {
				c.Header("Retry-After", strconv.FormatInt(int64(provider.RetryAfter/time.Second), 10))
			}
			code := "model_rate_limit"
			if provider.Uncertain {
				code = "model_rate_limit_uncertain"
			}
			Fail(c, 429, code, "Model provider rate limit or quota reached; retry later", false)
		case errors.As(err, &provider) && provider.Kind == "provider":
			c.Header("Retry-After", strconv.FormatInt(int64(10*time.Minute/time.Second), 10))
			Fail(c, http.StatusServiceUnavailable, "model_provider_unavailable", "Model provider unavailable; retry later", false)
		case errors.Is(err, modelbroker.ErrTurnFailed):
			Fail(c, 409, "model_retry", "Model turn failed and was settled; retry the turn", true)
		case errors.Is(err, modelbroker.ErrUncertain):
			Fail(c, 409, "model_uncertain", "Model usage is unresolved; review held allowance before retrying", false)
		case errors.Is(err, repair.ErrRunLimit):
			Fail(c, 409, "run_limit", "Run reached its model turn or time limit", false)
		case errors.Is(err, modelbroker.ErrUnavailable):
			Fail(c, 409, "model_unavailable", "Connect a verified model or agent with an available runner and maintenance allowance", false)
		case err != nil:
			budgetFailure(c, err)
		default:
			c.JSON(200, result)
		}
	})
}
