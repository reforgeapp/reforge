package gitea

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func TestWebhooksAuthenticateBodyAndKeepSourceIdentity(t *testing.T) {
	p, e := New(forge.Config{BaseURL: "https://forge.example", Client: &http.Client{}, Token: "token", WebhookSecret: "secret"})
	if e != nil {
		t.Fatal(e)
	}
	body := []byte(`{"repository":{"id":31,"full_name":"org/project"},"pull_request":{"number":8,"head":{"sha":"0123456789012345678901234567890123456789"}}}`)
	h := http.Header{"X-Gitea-Delivery": []string{"delivery-1"}, "X-Gitea-Event": []string{"pull_request"}}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	h.Set("X-Gitea-Signature", hex.EncodeToString(mac.Sum(nil)))
	event, e := p.DecodeEvent(h, body)
	if e != nil || event.Repository.NativeID != "31" || event.ChangeID != "8" {
		t.Fatalf("event=%+v error=%v", event, e)
	}
	body[len(body)-2] = ' '
	if _, e := p.DecodeEvent(h, body); e == nil {
		t.Fatal("tampered webhook accepted")
	}
	h.Del("X-Gitea-Signature")
	if e := p.VerifyWebhook(h, nil); e == nil {
		t.Fatal("unsigned webhook accepted")
	}
}
func TestTransportRedactsErrorsAndBoundsResponses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		handler   http.HandlerFunc
		kind      string
		uncertain bool
	}{{"error", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); fmt.Fprint(w, "secret-token-leak") }, "provider", true}, {"large", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", maxBody+1)) }, "response", true}, {"redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/leak", http.StatusFound)
	}, "provider", false}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()
			p, e := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "secret-token-leak"})
			if e != nil {
				t.Fatal(e)
			}
			e = p.request(context.Background(), "POST", "/test", nil, nil)
			pe, ok := e.(*domain.ProviderError)
			if !ok || pe.Kind != tc.kind || pe.Uncertain != tc.uncertain || strings.Contains(e.Error(), "secret-token") {
				t.Fatalf("unsafe error: %v", e)
			}
		})
	}
}
func TestRepositoryReplacementAndPaginationFailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/") {
			io.WriteString(w, `{"id":99,"full_name":"org/project"}`)
			return
		}
		fmt.Fprint(w, "[")
		for i := 0; i < 100; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":%d,"full_name":"org/p%d"}`, i+1, i)
		}
		fmt.Fprint(w, "]")
	}))
	defer server.Close()
	p, _ := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "token"})
	if _, e := p.GetRepository(context.Background(), forge.RepoRef{NativeID: "1", FullName: "org/project"}); e == nil {
		t.Fatal("replacement repository trusted")
	}
	if _, e := p.ListRepositories(context.Background(), forge.InventoryRequest{Cursor: "20", Limit: 100}); e == nil {
		t.Fatal("partial exhausted inventory marked complete")
	}
}
func TestCodeOwnersGiteaRegexAndEscapes(t *testing.T) {
	rules, e := ParseCodeOwners([]byte(".*\\.go @alice @org/team # comment\n!frontend/.* @other\npath\\ with\\ space @space\ndir/with\\#hash @hash\npath/\\\\.dot @dot\n"))
	if e != nil || len(rules) != 5 {
		t.Fatalf("rules=%+v error=%v", rules, e)
	}
	if rules[0].Pattern != `.*\.go` || !rules[1].Negative || rules[2].Pattern != "path with space" || rules[3].Pattern != "dir/with#hash" || rules[4].Pattern != `path/\.dot` {
		t.Fatalf("Gitea expressions changed %+v", rules)
	}
	for _, text := range []string{"[bad @user", ".* nobody", "! @user"} {
		if _, e := ParseCodeOwners([]byte(text)); e == nil {
			t.Fatalf("invalid rule accepted %q", text)
		}
	}
}
func TestBranchAuthorizationDefaultsDeny(t *testing.T) {
	p, _ := New(forge.Config{BaseURL: "https://forge.example", Client: &http.Client{}, Token: "token"})
	if _, e := p.UpdateAppBranch(context.Background(), forge.UpdateBranchRequest{}); e == nil {
		t.Fatal("branch mutation without persisted ownership accepted")
	}
	other, _ := New(forge.Config{BaseURL: "https://other.example", Client: &http.Client{}, Token: "token"})
	if _, e := p.WithProtectionReader(other); e == nil {
		t.Fatal("cross-origin inspector accepted")
	}
}

func TestTreeRejectsIncompleteAndRepeatedPages(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"incomplete", `{"total_count":2,"truncated":false,"tree":[{"path":"a","type":"blob","mode":"100644","sha":"0123456789012345678901234567890123456789"}]}`},
		{"repeated", `{"total_count":3,"truncated":true,"tree":[{"path":"a","type":"blob","mode":"100644","sha":"0123456789012345678901234567890123456789"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, tc.body) }))
			defer server.Close()
			p, _ := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "token"})
			if _, e := p.tree(context.Background(), forge.RepoRef{NativeID: "1", FullName: "org/project"}, "0123456789012345678901234567890123456789"); e == nil {
				t.Fatal("unverified partial tree accepted")
			}
		})
	}
}

func TestProtectionDoesNotApplyDifferentCaseRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/org/project":
			io.WriteString(w, `{"id":1,"full_name":"org/project","permissions":{"push":true},"allow_fast_forward_only_merge":true}`)
		case "/api/v1/user":
			io.WriteString(w, `{"id":2,"login":"bot","is_admin":false}`)
		case "/api/v1/repos/org/project/branch_protections":
			io.WriteString(w, `[{"rule_name":"Main","required_approvals":1,"block_on_outdated_branch":true}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	p, _ := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "token"})
	p, _ = p.WithProtectionReader(p)
	rules, err := p.ReadEffectiveRules(context.Background(), forge.RepoRef{NativeID: "1", FullName: "org/project"}, "main")
	if err != nil || rules.State != domain.Unknown || rules.Reason != "Target branch has no native protection" {
		t.Fatalf("incorrect native rule applied: %+v %v", rules, err)
	}
}
