package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"
)

type login struct {
	challenge string
	nonce     string
}

type provider struct {
	issuer  string
	client  string
	key     *rsa.PrivateKey
	mu      sync.Mutex
	pending map[string]login
}

func main() {
	oidcAddr := flag.String("oidc-addr", "127.0.0.1:5556", "OIDC issuer listen address")
	proxyAddr := flag.String("proxy-addr", "127.0.0.1:8443", "TLS reverse-proxy listen address")
	upstream := flag.String("upstream", "http://127.0.0.1:8080", "control-plane upstream origin")
	clientID := flag.String("client-id", "reforge-local", "OIDC client ID")
	caOut := flag.String("ca-out", "", "write the fixture CA certificate to this path")
	flag.Parse()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	caPEM, leaf, err := localCA()
	if err != nil {
		log.Fatal(err)
	}
	if *caOut != "" {
		if err = os.WriteFile(*caOut, caPEM, 0644); err != nil {
			log.Fatal(err)
		}
	}
	config := &tls.Config{Certificates: []tls.Certificate{leaf}, MinVersion: tls.VersionTLS12}
	p := &provider{issuer: "https://" + *oidcAddr, client: *clientID, key: key, pending: map[string]login{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("/keys", p.keys)
	mux.HandleFunc("/authorize", p.authorize)
	mux.HandleFunc("/token", p.token)
	go func() {
		log.Printf("oidc issuer %s", p.issuer)
		server := &http.Server{Addr: *oidcAddr, Handler: mux, TLSConfig: config}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			log.Fatal(err)
		}
	}()

	target, err := url.Parse(*upstream)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := &http.Server{Addr: *proxyAddr, Handler: proxy, TLSConfig: config}
	log.Printf("tls proxy https://%s -> %s", *proxyAddr, *upstream)
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
}

func (p *provider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"issuer": p.issuer, "authorization_endpoint": p.issuer + "/authorize", "token_endpoint": p.issuer + "/token", "jwks_uri": p.issuer + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"}})
}

func (p *provider) keys(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "local", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes())}}})
}

func (p *provider) authorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	redirect := query.Get("redirect_uri")
	if redirect == "" || query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	code := randomToken()
	log.Printf("authorize client=%s redirect=%s", query.Get("client_id"), redirect)
	p.mu.Lock()
	p.pending[code] = login{challenge: query.Get("code_challenge"), nonce: query.Get("nonce")}
	p.mu.Unlock()
	location, err := url.Parse(redirect)
	if err != nil {
		http.Error(w, "invalid redirect", http.StatusBadRequest)
		return
	}
	params := location.Query()
	params.Set("code", code)
	params.Set("state", query.Get("state"))
	location.RawQuery = params.Encode()
	http.Redirect(w, r, location.String(), http.StatusFound)
}

func (p *provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	p.mu.Lock()
	entry := p.pending[code]
	challenge := entry.challenge
	p.mu.Unlock()
	client := r.Form.Get("client_id")
	if client == "" {
		if user, _, ok := r.BasicAuth(); ok {
			client = user
		}
	}
	hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if challenge == "" || base64.RawURLEncoding.EncodeToString(hash[:]) != challenge || client != p.client {
		log.Printf("token rejected challenge=%v client=%s", challenge != "", client)
		http.Error(w, "invalid grant", http.StatusBadRequest)
		return
	}
	log.Printf("token issued for %s", client)
	payload, _ := json.Marshal(map[string]any{"iss": p.issuer, "sub": "local-owner", "aud": p.client, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": entry.nonce, "name": "Local Owner", "email": "owner@example.test"})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"local"}`))
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		http.Error(w, "sign failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"access_token": "local-access", "token_type": "Bearer", "expires_in": 3600, "id_token": unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func randomToken() string {
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func localCA() ([]byte, tls.Certificate, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "reforge-local-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "reforge-local"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})
	pair, err := tls.X509KeyPair(leafPEM, keyPEM)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	return caPEM, pair, nil
}
