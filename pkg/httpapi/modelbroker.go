package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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
		case errors.As(err, &provider) && (provider.Kind == "quota" || provider.Kind == "rate_limit" || provider.Kind == "rate_limited"):
			Fail(c, 429, "model_rate_limit", "Model provider rate limit or quota reached; retry later", false)
		case errors.Is(err, modelbroker.ErrTurnFailed):
			Fail(c, 409, "model_retry", "Model turn failed and was settled; retry the turn", true)
		case errors.Is(err, modelbroker.ErrUncertain):
			Fail(c, 409, "model_uncertain", "Model usage is unresolved; review held allowance before retrying", false)
		case errors.Is(err, repair.ErrRunLimit):
			Fail(c, 409, "run_limit", "Run reached its model turn or time limit", false)
		case errors.Is(err, modelbroker.ErrUnavailable):
			Fail(c, 409, "model_unavailable", "Use an approved direct API connection with a configured budget and assigned private runner", false)
		case err != nil:
			budgetFailure(c, err)
		default:
			c.JSON(200, result)
		}
	})
}
