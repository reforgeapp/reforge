package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reforge/internal/config"
	"reforge/internal/domain"
	"testing"
)

func TestIntegratedRouteRegistration(t *testing.T) {
	s := New(config.Config{WebDir: t.TempDir()}, nil)
	s.RegisterIdentity(nil)
	s.RegisterConnections(nil)
	s.RegisterPolicy(nil)
	s.RegisterWorkflow(nil)
	s.RegisterBudget(nil)
	s.RegisterInsights(nil)
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
