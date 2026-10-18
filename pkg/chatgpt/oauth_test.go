package chatgpt

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}
}

func TestOAuthPKCEIdentityGrantAndRefresh(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{ClientID: "oaiapp_fixture", RedirectURL: "https://app.example/auth/openai/callback", PlanUsageEnabled: true}
	nonce, verifier, state := Random(), Random(), Random()
	scopes := "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	claims := map[string]any{"iss": Issuer, "aud": config.ClientID, "sub": "owner-subject", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": nonce, "email": "owner@example.test"}
	sign := func() string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"fixture"}`))
		raw, _ := json.Marshal(claims)
		data := header + "." + base64.RawURLEncoding.EncodeToString(raw)
		digest := sha256.Sum256([]byte(data))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return data + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	refreshing := false
	tokenRequests := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "auth.openai.com" {
			t.Fatal("credential request escaped issuer")
		}
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			return reply(200, map[string]any{"issuer": Issuer, "authorization_endpoint": Issuer + "/api/accounts/authorize", "token_endpoint": Issuer + "/api/accounts/oauth/token", "jwks_uri": Issuer + "/.well-known/jwks.json", "revocation_endpoint": Issuer + "/oauth/revoke", "id_token_signing_alg_values_supported": []string{"RS256"}}), nil
		case "/.well-known/jwks.json":
			exponent := big.NewInt(int64(key.PublicKey.E)).Bytes()
			return reply(200, map[string]any{"keys": []any{map[string]string{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "fixture", "n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(exponent)}}}), nil
		case "/api/accounts/oauth/token":
			tokenRequests++
			if r.ParseForm() != nil || r.Form.Get("client_id") != config.ClientID || r.Form.Get("resource") != Resource || r.Form.Get("client_secret") != "" {
				t.Fatal("invalid OAuth token binding")
			}
			if refreshing {
				if r.Form.Get("refresh_token") != "refresh-one" || r.Form.Get("grant_type") != "refresh_token" || r.Form.Has("scope") {
					t.Fatal("refresh changed the grant")
				}
				return reply(200, map[string]any{"access_token": "access-two", "refresh_token": "refresh-two", "token_type": "Bearer", "expires_in": 3600}), nil
			}
			if r.Form.Get("code_verifier") != verifier || r.Form.Get("redirect_uri") != config.RedirectURL || r.Form.Get("code") != "authorization-code" {
				t.Fatal("PKCE transaction lost")
			}
			return reply(200, map[string]any{"access_token": "access-one", "refresh_token": "refresh-one", "id_token": sign(), "token_type": "Bearer", "expires_in": 3600, "scope": scopes}), nil
		case "/oauth/revoke":
			if r.ParseForm() != nil || r.Form.Get("token") != "refresh-two" || r.Form.Get("client_id") != config.ClientID || r.Form.Get("token_type_hint") != "refresh_token" {
				t.Fatal("revocation lost current session")
			}
			return reply(200, nil), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		return nil, errors.New("unexpected request")
	})}
	oauth, err := New(context.Background(), config, client)
	if err != nil {
		t.Fatal(err)
	}
	defer oauth.Close()
	location, _ := url.Parse(oauth.AuthorizationURL(state, nonce, verifier))
	query := location.Query()
	if query.Get("state") != state || query.Get("nonce") != nonce || query.Get("code_challenge") != Digest(verifier) || query.Get("code_challenge_method") != "S256" || !strings.Contains(query.Get("scope"), "chatgpt.tokens.use.direct") {
		t.Fatal("authorization missing consent or PKCE")
	}
	credentials, err := oauth.Exchange(context.Background(), "authorization-code", nonce, verifier)
	if err != nil || credentials.Subject != "owner-subject" {
		t.Fatalf("validated account not retained: %v", err)
	}
	refreshing = true
	renewed, err := oauth.Refresh(context.Background(), credentials)
	if err != nil || renewed.AccessToken != "access-two" || renewed.RefreshToken != "refresh-two" || renewed.Scopes != scopes || renewed.Subject != credentials.Subject {
		t.Fatalf("rotation failed: %v", err)
	}
	if err := oauth.Revoke(context.Background(), renewed); err != nil {
		t.Fatal(err)
	}
	refreshing = false
	for _, field := range []string{"nonce", "aud", "iss", "exp"} {
		original := claims[field]
		if field == "exp" {
			claims[field] = time.Now().Add(-time.Hour).Unix()
		} else {
			claims[field] = "wrong"
		}
		if _, err := oauth.Exchange(context.Background(), "authorization-code", nonce, verifier); !errors.Is(err, ErrAuthorization) {
			t.Fatalf("accepted invalid %s", field)
		}
		claims[field] = original
	}
	scopes = "openid profile email"
	if _, err := oauth.Exchange(context.Background(), "authorization-code", nonce, verifier); !errors.Is(err, ErrPermission) {
		t.Fatal("identity-only login authorized inference")
	}
	if tokenRequests != 7 {
		t.Fatalf("unexpected token request count %d", tokenRequests)
	}
	for _, value := range []any{credentials, config} {
		if strings.Contains(fmt.Sprintf("%+v %#v", value, value), "access-one") {
			t.Fatal("credential leaked through logging")
		}
	}
}

func TestRefreshRecoveryPreservesTemporaryFailures(t *testing.T) {
	current := Credentials{ClientID: "oaiapp_fixture", Subject: "subject", AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now(), Scopes: "resource.invoke chatgpt.tokens.use.direct"}
	for _, code := range []string{"invalid_grant", "refresh_token_reused", "refresh_token_expired"} {
		if _, err := refreshResponse(reply(400, map[string]string{"error": code}), current); !errors.Is(err, ErrRefresh) {
			t.Fatalf("terminal refresh error not recognized: %s", code)
		}
	}
	if _, err := refreshResponse(reply(503, map[string]string{"error": "temporarily_unavailable"}), current); !errors.Is(err, ErrUnavailable) {
		t.Fatal("temporary failure treated as revocation")
	}
	if _, err := refreshResponse(reply(200, map[string]any{"access_token": "renewed", "token_type": "Bearer", "expires_in": 3600, "scope": "openid"}), current); !errors.Is(err, ErrPermission) {
		t.Fatal("reduced grant authorized inference")
	}
}
