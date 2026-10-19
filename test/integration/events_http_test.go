package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/workflow"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEventStreamScopesAndLiveRevocation(t *testing.T) {
	ctx := context.Background()
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	_, owner := identityLogin(t, identity, server)
	org := owner.Organisations[0].ID
	repoA, repoB := domain.NewID(), domain.NewID()
	err := db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		for _, id := range []string{repoA, repoB} {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Stream fixture')`, org, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	user, cookie := fixtureIdentity(t, db)
	if _, err = identity.PutMember(ctx, owner, org, user, auth.Membership{Role: domain.Viewer, RepositoryIDs: []string{repoA}}, 0, "stream"); err != nil {
		t.Fatal(err)
	}
	session, err := identity.Authenticate(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	emit := func(repo, label string) {
		t.Helper()
		err := db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			body, _ := json.Marshal(map[string]string{"label": label})
			return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: "fixture", AggregateType: "task", AggregateID: domain.NewID(), AggregateVersion: 1, DataVersion: 1, RequestID: "stream", Data: body})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	emit(repoB, "hidden-tenant-scope")
	emit(repoA, "visible-scoped-event")
	service := workflow.New(db, identity, nil)
	server.RegisterWorkflow(service)
	httpServer := httptest.NewServer(server.Router)
	defer httpServer.Close()
	request, err := http.NewRequest("GET", httpServer.URL+"/api/v1/orgs/"+org+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "127.0.0.1:8080"
	request.AddCookie(cookie)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("event transport unavailable")
	}
	lines := make(chan string, 20)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") || strings.HasPrefix(line, "event:") {
				lines <- line
			}
		}
		close(lines)
	}()
	deadline := time.After(3 * time.Second)
	seen := false
	for !seen {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before event")
			}
			if strings.Contains(line, "hidden-tenant-scope") {
				t.Fatal("unauthorised repository leaked")
			}
			seen = strings.Contains(line, "visible-scoped-event")
		case <-deadline:
			t.Fatal("scoped event missing")
		}
	}
	if err = identity.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	deadline = time.After(3 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream omitted revoked state")
			}
			if strings.Contains(line, "hidden-tenant-scope") {
				t.Fatal("scope leaked")
			}
			if strings.Contains(line, "access_revoked") {
				return
			}
		case <-deadline:
			t.Fatal("revoked session retained event stream")
		}
	}
}
