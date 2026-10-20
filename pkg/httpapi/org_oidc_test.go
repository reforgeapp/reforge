package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func TestOrgOIDCActivationRequiresCurrentVerification(t *testing.T) {
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	oidcFailure(context, auth.ErrOIDCActivation)
	if writer.Code != 409 {
		t.Fatalf("activation status = %d", writer.Code)
	}
	var response domain.Error
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil || response.Code != "activation_requires_verification" || response.Message == "" {
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

func TestOrgLoginFailureUsesOIDCStatuses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
		retry  string
	}{
		{auth.ErrOIDCProbeBusy, 429, "probe_capacity", "1"},
		{auth.ErrOIDCUnavailable, 503, "secret_storage_unavailable", ""},
	} {
		writer := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(writer)
		loginFailure(context, true, tc.err)
		var response domain.Error
		if writer.Code != tc.status || writer.Header().Get("Retry-After") != tc.retry || json.Unmarshal(writer.Body.Bytes(), &response) != nil || response.Code != tc.code {
			t.Fatalf("org login error %v mapped to %d %q", tc.err, writer.Code, response.Code)
		}
	}
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	loginFailure(context, false, auth.ErrOIDCProbeBusy)
	if writer.Code != 500 {
		t.Fatalf("global login error mapping changed: %d", writer.Code)
	}
}
