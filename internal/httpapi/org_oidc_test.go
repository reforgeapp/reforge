package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"reforge/internal/auth"
	"reforge/internal/domain"
)

func TestOrgOIDCActivationBlockedResponseIsActionable(t *testing.T) {
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	oidcFailure(context, auth.ErrOIDCNotReady)
	if writer.Code != 409 {
		t.Fatalf("activation status = %d", writer.Code)
	}
	var response domain.Error
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil || response.Code != "activation_unavailable" || response.Message == "" {
		t.Fatal("activation block response is not actionable")
	}
}

func TestOrgOIDCProbeCooldownReturnsRetryAfter(t *testing.T) {
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	oidcFailure(context, &auth.ProbeCooldownError{RetryAfter: 17})
	var response domain.Error
	if writer.Code != 429 || writer.Header().Get("Retry-After") != "17" || json.Unmarshal(writer.Body.Bytes(), &response) != nil || response.Code != "probe_cooldown" {
		t.Fatal("probe cooldown response omitted rate limit guidance")
	}
	writer = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(writer)
	oidcFailure(context, auth.ErrOIDCProbeBusy)
	if writer.Code != 429 || writer.Header().Get("Retry-After") != "1" {
		t.Fatal("concurrency limit did not return retry guidance")
	}
}
