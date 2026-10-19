package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

const testHead = "1111111111111111111111111111111111111111"
const testBase = "2222222222222222222222222222222222222222"
const testCommit = "3333333333333333333333333333333333333333"

var testRepo = forge.RepoRef{NativeID: "1", FullName: "acme/repo"}

func jsonResponse(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}
}
func fixtureProvider(t *testing.T, fn roundTripFunc) *Provider {
	t.Helper()
	p, err := New(forge.Config{BaseURL: "https://github.example/api/v3", Token: "fixture-token", Client: fn})
	if err != nil {
		t.Fatal(err)
	}
	p = authorizedFixture(p)
	p, err = p.WithGraphQL("https://github.example/api/graphql", fn)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func pullFixture(actor int64, id int) map[string]any {
	repo := map[string]any{"id": 1, "full_name": "acme/repo"}
	return map[string]any{"number": id, "body": "<!-- reforge-operation-id:op-1 -->", "state": "open", "mergeable_state": "clean", "user": map[string]any{"id": actor, "type": "Bot", "login": "reforge[bot]"}, "head": map[string]any{"sha": testHead, "ref": "reforge/fix", "repo": repo}, "base": map[string]any{"sha": "stale-target", "ref": "main", "repo": repo}}
}
func TestInstallationJWTIdentityRefreshAndRedaction(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	var minted atomic.Int32
	var wrongBot atomic.Bool
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/app") {
			parts := strings.Split(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "), ".")
			if len(parts) != 3 {
				t.Error("missing App JWT")
				return jsonResponse(401, nil), nil
			}
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], sig); err != nil {
				t.Error("invalid App signature")
			}
			raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims struct {
				Issuer  string `json:"iss"`
				Issued  int64  `json:"iat"`
				Expires int64  `json:"exp"`
			}
			_ = json.Unmarshal(raw, &claims)
			if claims.Issuer != "42" || claims.Issued > time.Now().Unix() || claims.Expires <= time.Now().Unix() || claims.Expires > time.Now().Add(10*time.Minute).Unix() {
				t.Error("JWT binding or lifetime invalid")
			}
		}
		switch req.URL.Path {
		case "/api/v3/app":
			return jsonResponse(200, map[string]any{"id": 42, "slug": "reforge"}), nil
		case "/api/v3/app/installations/18":
			return jsonResponse(200, map[string]any{"id": 18, "app_id": 99}), nil
		case "/api/v3/app/installations/17":
			return jsonResponse(200, map[string]any{"id": 17, "app_id": 42}), nil
		case "/api/v3/app/installations/17/access_tokens":
			minted.Add(1)
			return jsonResponse(201, map[string]any{"token": "secret-installation-token", "expires_at": time.Now().Add(time.Hour)}), nil
		case "/api/v3/users/reforge[bot]":
			if req.Header.Get("Authorization") != "Bearer secret-installation-token" {
				t.Error("bot lookup did not use installation token")
			}
			actor := 2
			if wrongBot.Load() {
				actor = 3
			}
			return jsonResponse(200, map[string]any{"id": actor, "type": "Bot", "login": "reforge[bot]"}), nil
		default:
			t.Errorf("unexpected App path %s", req.URL.Path)
			return jsonResponse(404, nil), nil
		}
	})
	app := AppConfig{AppID: "42", InstallationID: "17", PrivateKeyPEM: encoded}
	p, err := NewApp(context.Background(), forge.Config{BaseURL: "https://github.example/api/v3", Client: client}, app)
	if err != nil {
		t.Fatal(err)
	}
	p.app.mu.Lock()
	p.app.expires = time.Now()
	p.app.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			actor, err := p.authenticatedBot(context.Background())
			if err != nil || actor != "2" {
				t.Errorf("refresh identity %q %v", actor, err)
			}
		}()
	}
	wg.Wait()
	if minted.Load() != 2 {
		t.Fatalf("concurrent refresh minted %d tokens", minted.Load())
	}
	wrongBot.Store(true)
	p.app.mu.Lock()
	p.app.expires = time.Now()
	p.app.mu.Unlock()
	if _, err = p.authenticatedBot(context.Background()); err == nil {
		t.Fatal("changed immutable bot identity accepted")
	}
	raw, _ := json.Marshal(app)
	for _, value := range []string{string(raw), fmt.Sprintf("%+v %#v", app, app)} {
		if strings.Contains(value, "PRIVATE KEY") {
			t.Fatal("private key exposed")
		}
	}
	_, err = NewApp(context.Background(), forge.Config{BaseURL: "https://github.example/api/v3", Client: client}, AppConfig{AppID: "42", InstallationID: "18", PrivateKeyPEM: encoded})
	if err == nil {
		t.Fatal("unavailable installation accepted")
	}
}
func TestInstallationInventoryEmptyFilesAndBoundedPagination(t *testing.T) {
	p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v3/installation/repositories":
			return jsonResponse(200, map[string]any{"total_count": 1, "repositories": []any{map[string]any{"id": 1, "full_name": "acme/repo"}}}), nil
		case "/api/v3/repos/acme/repo/contents/empty":
			return jsonResponse(200, map[string]any{"type": "file", "encoding": "base64", "content": "", "path": "empty", "sha": testCommit}), nil
		default:
			return jsonResponse(404, nil), nil
		}
	})
	page, err := p.ListRepositories(context.Background(), forge.InventoryRequest{Namespace: "installation"})
	if err != nil || len(page.Items) != 1 || !page.Complete {
		t.Fatalf("installation inventory: %+v %v", page, err)
	}
	file, err := p.ReadFileAtRef(context.Background(), testRepo, "empty", testHead)
	if err != nil || len(file.Content) != 0 {
		t.Fatalf("empty file: %+v %v", file, err)
	}
	for _, next := range []int{0, 1, 3, maxPages + 1} {
		header := make(http.Header)
		header.Set("Link", fmt.Sprintf(`<https://github.example/api/v3/installation/repositories?page=%d>; rel="next"`, next))
		if _, err := p.nextCursor(header, 1); err == nil {
			t.Fatalf("nonadvancing or skipped page %d accepted", next)
		}
	}
}
func TestOperationReconciliationUsesAuthenticatedActorAndRejectsDuplicates(t *testing.T) {
	for _, ownCount := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(ownCount), func(t *testing.T) {
			p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/git/ref/heads/main") {
					return jsonResponse(200, map[string]any{"object": map[string]string{"sha": testBase}}), nil
				}
				rows := []any{pullFixture(999, 8)}
				for i := 0; i < ownCount; i++ {
					rows = append(rows, pullFixture(2, 10+i))
				}
				return jsonResponse(200, rows), nil
			})
			found, err := p.FindChangeByOperation(context.Background(), testRepo, "op-1", "reforge/fix", "main")
			if ownCount == 0 && (err != nil || found != nil) || ownCount == 1 && (err != nil || found == nil || found.ID != "10" || found.TargetSHA != testBase || found.Repository.NativeID != "1") || ownCount == 2 && err == nil {
				t.Fatalf("found=%+v err=%v", found, err)
			}
		})
	}
}
func TestBranchPublicationUsesImmutableRefAndExactHead(t *testing.T) {
	for _, mode := range []string{"create", "update", "head_changed", "revoked", "wrong_repository"} {
		t.Run(mode, func(t *testing.T) {
			var mutations, guards int
			p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" {
					return jsonResponse(200, map[string]any{"object": map[string]string{"sha": testCommit}}), nil
				}
				var body struct {
					Query     string                     `json:"query"`
					Variables map[string]json.RawMessage `json:"variables"`
				}
				_ = json.NewDecoder(req.Body).Decode(&body)
				if strings.HasPrefix(body.Query, "query") {
					var ref any
					if mode != "create" && mode != "revoked" {
						ref = map[string]any{"id": "REF_fixed", "target": map[string]string{"oid": testHead}}
					}
					id := 1
					if mode == "wrong_repository" {
						id = 9
					}
					return jsonResponse(200, map[string]any{"data": map[string]any{"repository": map[string]any{"id": "REPO_fixed", "databaseId": id, "ref": ref}}}), nil
				}
				mutations++
				var input map[string]any
				_ = json.Unmarshal(body.Variables["input"], &input)
				if strings.Contains(body.Query, "CreateRefInput") {
					if guards != 1 || input["repositoryId"] != "REPO_fixed" || input["oid"] != testHead {
						t.Errorf("unguarded branch creation %v", input)
					}
					return jsonResponse(200, map[string]any{"data": map[string]any{"createRef": map[string]any{"ref": map[string]any{"id": "REF_fixed", "target": map[string]string{"oid": testHead}}}}}), nil
				}
				branch := input["branch"].(map[string]any)
				if guards != 2 || input["expectedHeadOid"] != testHead || branch["id"] != "REF_fixed" || len(branch) != 1 {
					t.Errorf("unguarded or name-bound commit %v", input)
				}
				if mode == "head_changed" {
					return jsonResponse(200, map[string]any{"errors": []any{map[string]string{"type": "STALE_DATA"}}}), nil
				}
				return jsonResponse(200, map[string]any{"data": map[string]any{"createCommitOnBranch": map[string]any{"commit": map[string]string{"oid": testCommit}}}}), nil
			})
			p = p.WithBranchAuthorizer(func(context.Context, forge.UpdateBranchRequest) error {
				guards++
				if mode == "revoked" && guards == 2 {
					return errors.New("revoked")
				}
				return nil
			})
			in := forge.UpdateBranchRequest{Repository: testRepo, Branch: "reforge/fix", BaseSHA: testHead, Message: "Repair", OperationID: "op-1", Edits: []forge.FileEdit{{Path: "empty", Content: []byte{}}}}
			if mode != "create" && mode != "revoked" {
				in.ExpectedOldSHA = testHead
			}
			got, err := p.UpdateAppBranch(context.Background(), in)
			if mode == "create" || mode == "update" {
				if err != nil || got != testCommit {
					t.Fatalf("commit %q %v", got, err)
				}
			} else if err == nil {
				t.Fatal("unsafe commit succeeded")
			}
			if mode == "revoked" && mutations != 1 || mode == "wrong_repository" && mutations != 0 {
				t.Fatalf("mutations=%d", mutations)
			}
		})
	}
}
func TestMutationTransportFailureIsUncertainAndSanitized(t *testing.T) {
	p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("transport exposes secret-installation-token")
	})
	_, _, _, err := p.request(context.Background(), "POST", []string{"repos", "acme", "repo", "pulls"}, nil, &requestBody{data: []byte(`{}`)})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain || strings.Contains(err.Error(), "secret-installation-token") {
		t.Fatalf("mutation error %+v", err)
	}
	if _, err := p.WithGraphQL("https://different.example/graphql", p.config.Client); err == nil {
		t.Fatal("cross-origin GraphQL accepted")
	}
}

func TestNativeMutationsRequirePersistedAuthority(t *testing.T) {
	calls := 0
	p := fixtureProvider(t, func(*http.Request) (*http.Response, error) { calls++; return jsonResponse(500, nil), nil })
	p = p.WithBranchAuthorizer(nil).WithChangeAuthorizer(nil).WithReviewAuthorizer(nil)
	if _, err := p.UpdateAppBranch(context.Background(), forge.UpdateBranchRequest{}); err == nil {
		t.Fatal("unguarded branch mutation")
	}
	if _, err := p.CreateChange(context.Background(), forge.CreateChangeRequest{}); err == nil {
		t.Fatal("unguarded PR mutation")
	}
	if err := p.RequestReview(context.Background(), testRepo, "7", []string{"reviewer"}); err == nil {
		t.Fatal("unguarded review mutation")
	}
	if _, err := p.RequestNativeMergeOrQueue(context.Background(), forge.MergeRequest{}); err == nil {
		t.Fatal("unguarded merge mutation")
	}
	if calls != 0 {
		t.Fatalf("unguarded outbound calls=%d", calls)
	}
}

func TestCapabilityProbeRequiresAuthenticatedAccess(t *testing.T) {
	for _, app := range []bool{false, true} {
		p, err := New(forge.Config{BaseURL: "https://github.example/api/v3", Token: "revoked", Client: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/v3/meta" {
				return jsonResponse(200, map[string]any{}), nil
			}
			return jsonResponse(401, nil), nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		if app {
			p = authorizedFixture(p)
		}
		if _, err = p.ProbeCapabilities(context.Background()); err == nil {
			t.Fatal("public metadata certified revoked credentials")
		}
	}
}
