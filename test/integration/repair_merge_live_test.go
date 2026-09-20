package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/maintenance/repair"
	"reforge/internal/mergecontrol"
	"reforge/internal/policy"
	"reforge/internal/providers"
)

func verifyLiveProtectedMerge(t *testing.T, ctx context.Context, f *inventoryFixture, policies *policy.Service, discoveries *discovery.Service, reader *providers.Service, route *connections.Route, connection connections.Connection, repositoryID string, repo forge.RepoRef, run repair.Run, document policy.Policy, native func(string, string, any, any), root string) {
	t.Helper()
	secret := func(actor string) string {
		raw, err := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-"+actor+".token"))
		if err != nil {
			t.Fatal("local native merge credential missing")
		}
		defer clear(raw)
		return strings.TrimSpace(string(raw))
	}
	native("PUT", "/collaborators/reforge-reviewer", map[string]string{"permission": "write"}, nil)
	native("PATCH", "", map[string]any{"allow_fast_forward_only_merge": true}, nil)
	native("POST", "/branch_protections", map[string]any{"rule_name": "main", "enable_push": false, "required_approvals": 1, "dismiss_stale_approvals": true, "block_on_outdated_branch": true, "block_on_rejected_reviews": true, "block_on_official_review_requests": true, "block_admin_merge_override": true, "enable_status_check": false}, nil)
	var err error
	connection, err = f.connections.Rotate(ctx, f.owner, f.org, connection.ID, connection.Version, secret("reviewer"), "native-merge")
	if err != nil {
		t.Fatal(err)
	}
	connection, err = f.connections.Test(ctx, f.owner, f.org, connection.ID, connection.Version, "native-merge")
	if err != nil || connection.State != "healthy" {
		t.Fatalf("merge actor connection: %v %s", err, connection.State)
	}
	inspector, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "Native merge protection reader", Endpoint: connection.Endpoint, Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: secret("inspector"), PrivateRoute: route}, "native-merge")
	if err != nil {
		t.Fatal(err)
	}
	inspector, err = f.connections.Test(ctx, f.owner, f.org, inspector.ID, inspector.Version, "native-merge")
	if err != nil || inspector.State != "healthy" {
		t.Fatalf("protection inspector: %v %s", err, inspector.State)
	}
	if _, err = discoveries.PutConfig(ctx, f.owner, f.org, repositoryID, discovery.Config{MergeAuthority: "reforge"}, 0, "native-merge"); err != nil {
		t.Fatal(err)
	}
	document.Allow.MergeMethods = []string{"fast-forward-only"}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "native-merge", "native-merge")
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := policies.Simulate(ctx, f.owner, f.org, version.ID, repositoryID, "", policy.Input{Action: policy.Merge})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, repositoryID, "", 1, simulation.Hash, "native-merge", "native-merge"); err != nil {
		t.Fatal(err)
	}
	merges := mergecontrol.New(f.db, f.identity, f.connections, policies, reader)
	qualification := mergecontrol.Qualification{Provider: "gitea", ServerVersion: connection.ServerVersion, ConnectionVersion: connection.Version, InspectorVersion: inspector.Version, EvidenceReference: "local-gitea-1.27.3-protection-contract", EvidenceSHA256: fileDigest(t, filepath.Join(root, "test/forge/gitea/contract_test.go")), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), ExactHead: true, StrictTarget: true}
	cfg, err := merges.PutConfig(ctx, f.owner, f.org, repositoryID, mergecontrol.Configuration{Enabled: true, InspectorConnectionID: inspector.ID, CheckPublishers: map[string]string{}, CooperationReference: "disposable fixture has no bot automerge", Qualification: qualification}, 0, "native-merge")
	if err != nil {
		t.Fatal(err)
	}
	inspect := func() mergecontrol.Gate {
		t.Helper()
		gate, err := merges.Inspect(ctx, f.owner, f.org, repositoryID, run.Change.ID, "fast-forward-only", "native-merge")
		if err != nil {
			t.Fatal(err)
		}
		return gate
	}
	blocked := inspect()
	if blocked.Decision.Outcome == "allow" {
		t.Fatal("missing native approval allowed merge")
	}
	if _, err = merges.Request(ctx, f.owner, f.org, blocked.ID, domain.NewID(), "native-merge"); err == nil {
		t.Fatal("blocked preview admitted")
	}
	body, _ := json.Marshal(map[string]string{"commit_id": run.CandidateSHA, "event": "APPROVED", "body": "Disposable native contract approval"})
	req, err := http.NewRequestWithContext(ctx, "POST", connection.Endpoint+"/api/v1/repos/"+repo.FullName+"/pulls/"+run.Change.ID+"/reviews", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+secret("reviewer"))
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal("native approval request failed")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("native approval HTTP%d", response.StatusCode)
	}
	gate := inspect()
	if gate.Decision.Outcome != "allow" {
		t.Fatalf("native approved preview blocked: %+v native=%+v rules=%+v", gate.Decision, gate.Snapshot.Native, gate.Snapshot.Rules)
	}
	if _, err = merges.PutConfig(ctx, f.owner, f.org, repositoryID, cfg, cfg.Version, "native-merge"); err != nil {
		t.Fatal(err)
	}
	if _, err = merges.Request(ctx, f.owner, f.org, gate.ID, domain.NewID(), "native-merge"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale configuration admitted: %v", err)
	}
	gate = inspect()
	key := domain.NewID()
	op, err := merges.Request(ctx, f.owner, f.org, gate.ID, key, "native-merge")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10 && op.State != "merged"; i++ {
		if op.State != "reconciling" && op.State != "dispatching" {
			t.Fatalf("native merge state=%s reason=%s", op.State, op.Reason)
		}
		time.Sleep(100 * time.Millisecond)
		op, err = merges.Observe(ctx, f.org, op.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if op.State != "merged" || op.NativeResult == nil || op.NativeResult.MergeSHA != run.CandidateSHA {
		t.Fatalf("canonical protected merge not observed: %+v", op)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE merge_operations SET state='reconciling',native_result=NULL,reason='Injected loss after native merge',version=version+1 WHERE org_id=$1 AND id=$2`, f.org, op.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	merges = mergecontrol.New(f.db, f.identity, f.connections, policies, reader)
	op, err = merges.Observe(ctx, f.org, op.ID)
	if err != nil || op.State != "merged" || op.NativeResult == nil || op.NativeResult.MergeSHA != run.CandidateSHA {
		t.Fatalf("restart canonical observation: %+v %v", op, err)
	}
	replayed, err := merges.Request(ctx, f.owner, f.org, gate.ID, key, "native-merge")
	if err != nil || replayed.ID != op.ID || replayed.State != "merged" {
		t.Fatalf("idempotent merge replay: %+v %v", replayed, err)
	}
	if _, err = merges.Get(ctx, f.owner, domain.NewID(), op.ID); err == nil {
		t.Fatal("cross-tenant merge read allowed")
	}
	t.Logf("protected native merge observed operation=%s head=%s", op.ID, op.NativeResult.MergeSHA)
}
