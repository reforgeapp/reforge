package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
	"testing"
)

func TestPolicyHTTPActivationAndSimulation(t *testing.T) {
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, identity, server)
	orgID := domain.NewID()
	err := db.Tenant(context.Background(), orgID, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO organisations(id,name) VALUES($1,'Policy HTTP contract')`, orgID); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, orgID, owner.User.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := policy.New(db, identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	server.RegisterPolicy(svc)
	base := "/api/v1/orgs/" + orgID + "/policies"
	headers := map[string]string{"Origin": "http://127.0.0.1:8080", "X-CSRF-Token": owner.CSRFToken, "Content-Type": "application/json", "If-Match": "\"0\""}
	body := fmt.Sprintf(`{"scope":{"kind":"organisation","id":%q},"policy":{"schema":"maintenance/v1","deny":["merge"]},"reason":"Review activation"}`, orgID)
	response := identityRequest(server, "POST", base+"/versions", body, cookie, headers)
	if response.Code != 201 {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var v policy.Version
	if err = json.Unmarshal(response.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	response = identityRequest(server, "GET", base+"/effective", "", cookie, nil)
	var before policy.Resolved
	json.Unmarshal(response.Body.Bytes(), &before)
	if response.Code != 200 || len(before.Problems) == 0 {
		t.Fatal("inactive candidate authorised policy")
	}
	response = identityRequest(server, "POST", base+"/versions/"+v.ID+"/simulate", `{"input":{"action":"merge"}}`, cookie, headers)
	var simulation policy.Simulation
	if err = json.Unmarshal(response.Body.Bytes(), &simulation); err != nil || response.Code != 200 || simulation.Decision.Outcome != "deny" {
		t.Fatalf("simulation: %d %s", response.Code, response.Body.String())
	}
	activation := fmt.Sprintf(`{"simulation_hash":%q,"reason":"Activate reviewed restriction"}`, simulation.Hash)
	response = identityRequest(server, "POST", base+"/versions/"+v.ID+"/activate", activation, cookie, headers)
	if response.Code != 200 || response.Header().Get("ETag") != "\"1\"" {
		t.Fatalf("activate: %d %s", response.Code, response.Body.String())
	}
	response = identityRequest(server, "POST", base+"/versions/"+v.ID+"/activate", activation, cookie, headers)
	if response.Code != 409 {
		t.Fatal("stale activation accepted")
	}
	response = identityRequest(server, "GET", base+"/versions", "", cookie, nil)
	var page domain.Page[policy.Version]
	json.Unmarshal(response.Body.Bytes(), &page)
	if response.Code != 200 || len(page.Items) != 1 {
		t.Fatal("policy history unavailable")
	}
	response = identityRequest(server, "POST", base+"/versions", body, cookie, nil)
	if response.Code != 403 {
		t.Fatal("policy write omitted CSRF")
	}
}
