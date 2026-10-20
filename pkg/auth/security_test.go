package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestProductionCookiesAndOrigin(t *testing.T) {
	origin, _ := url.Parse("https://reforge.example")
	service := &Service{origin: origin, cfg: Config{PublicURL: origin.String()}}
	recorder := httptest.NewRecorder()
	service.SetSession(recorder, "opaque")
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("session cookie missing")
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-reforge_session" || cookie.Domain != "" || cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe session cookie")
	}
	binding := service.cookie(service.oidcCookieName(), "browser", 600)
	if binding.Name != "__Host-reforge_oidc" || binding.Domain != "" || binding.Path != "/" || !binding.Secure {
		t.Fatal("OIDC cookie allows sibling-domain injection")
	}
	request := httptest.NewRequest("POST", "https://reforge.example/auth/logout", nil)
	request.Header.Set("Origin", origin.String())
	request.Header.Set("X-CSRF-Token", "token")
	if err := service.CheckRequest(request, "token"); err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://sibling.example")
	if err := service.CheckRequest(request, "token"); err != ErrForbidden {
		t.Fatal("cross-origin write accepted")
	}
	request.Header.Set("Origin", origin.String())
	request.Host = "reforge.example.attacker.test"
	if err := service.CheckRequest(request, "token"); err != ErrForbidden {
		t.Fatal("untrusted Host accepted")
	}
}
