package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/config"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func TestIntegratedRouteRegistration(t *testing.T) {
	s := New(config.Config{WebDir: t.TempDir()}, nil)
	s.RegisterIdentity(nil)
	s.RegisterConnections(nil)
	s.RegisterNativeAgents(nil)
	s.RegisterPolicy(nil)
	s.RegisterWorkflow(nil)
	s.RegisterBudget(nil)
	s.RegisterInsights(nil)
	s.RegisterCampaigns(nil)
	s.RegisterRunner(nil)
	s.RegisterPrivateConnector(nil)
	s.RegisterInventory(nil)
	s.RegisterDiscovery(nil)
	s.RegisterModelBroker(nil)
	s.RegisterRepair(nil)
	w := httptest.NewRecorder()
	s.Router.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("registered application health: %d", w.Code)
	}
}

func TestDiscoveryFailureMapsTransientProviderToRetryable503(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_id", "request")
	discoveryFailure(c, &domain.ProviderError{Kind: "transient", Message: "request failed"})
	var body domain.Error
	if w.Code != http.StatusServiceUnavailable || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Code != "provider_unavailable" || !body.Retryable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Set("request_id", "request")
	discoveryFailure(c, errors.Join(privateconnector.ErrUnavailable, errors.New("private transport")))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("private unavailable status=%d", w.Code)
	}
}

func TestRequestBoundaryAndSPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<main>Reforge</main>"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(config.Config{WebDir: dir, Development: true, FixtureAuth: true}, nil)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/healthz", 200}, {"/readyz", 503}, {"/org/example/repositories", 200}, {"/api/v1/missing", 404}, {"/runner/v1/missing", 404}, {"/missing.js", 404}, {"/../go.mod", 404},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("X-Request-ID", "attacker\nvalue")
		w := httptest.NewRecorder()
		s.Router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d body %s", tc.path, w.Code, w.Body.String())
		}
		if w.Header().Get("X-Request-ID") == "" || w.Header().Get("X-Request-ID") == r.Header.Get("X-Request-ID") {
			t.Fatal("untrusted request id accepted")
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("CSP missing")
		}
		if tc.status >= 400 {
			var body domain.Error
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code == "" || body.RequestID == "" {
				t.Fatal("unstructured failure")
			}
		}
	}
}
