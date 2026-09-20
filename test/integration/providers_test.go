package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reforge/internal/forge"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/config"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/httpapi"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/runner"
	"reforge/internal/secrets"
	"reforge/internal/source"
	"reforge/internal/workflow"
)

func TestPrivateConnectionProbeUsesEnrolledRunnerAndVault(t *testing.T) {
	if os.Getenv("REFORGE_PRIVATE_GITEA_TEST") != "1" {
		t.Skip("requires explicit disposable Gitea fixture")
	}
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	cookie, session := identityLogin(t, identity, server)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	org := domain.NewID()
	if err := db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Private inventory fixture')`, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, session.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	vault, err := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	svc := connections.New(db, identity, vault, true)
	jobs := workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return "", errors.New("no repair authority")
	})
	runners := runner.New(db, identity, jobs, nil)
	svc.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, session, org, "", runner.PoolInput{Name: "Private onboarding " + domain.NewID(), RepositoryIDs: []string{}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, session, org, pool.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "private-onboarding")
	if err != nil {
		t.Fatal(err)
	}
	connector, err := privateconnector.New(privateconnector.Config{Authenticate: runners.AuthenticateSupervisor, Development: true, TTL: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	providers.Factory{Development: true}.Register(svc)
	providers.RegisterPrivate(svc, connector, runners)
	server.RegisterConnections(svc)
	transport := httptest.NewUnstartedServer(nil)
	privateAPI := httpapi.New(config.Config{PublicURL: "http://" + transport.Listener.Addr().String(), Development: true}, db)
	privateAPI.RegisterPrivateConnector(connector)
	transport.Config.Handler = privateAPI.Router
	transport.Start()
	defer transport.Close()
	client, err := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: transport.URL, Credential: supervisor.Token, Target: privateconnector.Target{OrgID: org, RunnerID: supervisor.Runner.ID}, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	token, err := os.ReadFile(filepath.Join(os.Getenv("REFORGE_TEST_ROOT"), ".local/gitea/reforge-bot.token"))
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimSpace(string(token))
	clear(token)
	connection, err := svc.Create(ctx, session, org, connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "Private onboarding", Endpoint: "http://127.0.0.1:53000", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: secret, PrivateRoute: &connections.Route{RunnerID: supervisor.Runner.ID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		resolved, err := svc.ResolveTx(ctx, tx, org, connection.ID, supervisor.Runner.ID)
		if err != nil {
			return err
		}
		if resolved.Client != nil {
			t.Fatal("control plane received private destination client")
		}
		if _, err = (providers.Factory{Development: true}).Forge(ctx, resolved); err == nil {
			t.Fatal("factory allowed direct private connection")
		}
		_, err = svc.ResolveTx(ctx, tx, org, connection.ID, domain.NewID())
		if !errors.Is(err, connections.ErrRunnerRequired) {
			t.Fatal("wrong private runner resolved")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- client.RunOnce(ctx) }()
	headers := map[string]string{"Origin": "http://127.0.0.1:8080", "X-CSRF-Token": session.CSRFToken, "If-Match": "\"1\"", "Content-Type": "application/json"}
	response := identityRequest(server, "POST", "/api/v1/orgs/"+org+"/connections/"+connection.ID+"/test", "", cookie, headers)
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 {
		t.Fatalf("private probe HTTP%d: %s", response.Code, response.Body.String())
	}
	var verified connections.Connection
	if json.Unmarshal(response.Body.Bytes(), &verified) != nil || verified.State != "healthy" || verified.ServerVersion != "1.27.3" || strings.Contains(response.Body.String(), secret) {
		t.Fatal("private connection did not verify safely")
	}
	reader := providers.New(db, svc, connector, runners, true)
	read := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeInventory, Inventory: &privateconnector.InventoryArgs{Limit: 100}}
	authorized := 0
	go func() { finished <- client.RunOnce(ctx) }()
	page, readErr := reader.Read(ctx, org, connection.ID, read, func(ctx context.Context, tx pgx.Tx, current connections.Connection) error {
		authorized++
		if current.ID != connection.ID || current.Version != verified.Version || current.OrgID != org {
			return auth.ErrForbidden
		}
		return nil
	})
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if readErr != nil || authorized != 1 || page.Inventory == nil {
		t.Fatalf("private inventory authorization/result: %v, calls %d", readErr, authorized)
	}
	repository := localPrivateRepository(t)
	go func() { finished <- client.RunOnce(ctx) }()
	resolved, resolveErr := reader.Read(ctx, org, connection.ID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: repository, Ref: "main"}}, func(context.Context, pgx.Tx, connections.Connection) error { return nil })
	if err = <-finished; err != nil || resolveErr != nil {
		t.Fatalf("private pinned ref: %v %v", err, resolveErr)
	}
	sourceReader := reader.SourceReader(org, connection.ID, func(context.Context, pgx.Tx, connections.Connection) error { return nil })
	manifestRead, fileRead := sourceReader.Manifest, sourceReader.File
	sourceReader.Manifest = func(ctx context.Context, repo forge.RepoRef, commit string) (forge.SourceManifest, error) {
		go func() { finished <- client.RunOnce(ctx) }()
		manifest, err := manifestRead(ctx, repo, commit)
		return manifest, errors.Join(err, <-finished)
	}
	sourceReader.File = func(ctx context.Context, repo forge.RepoRef, path, commit string) (forge.File, error) {
		go func() { finished <- client.RunOnce(ctx) }()
		file, err := fileRead(ctx, repo, path, commit)
		return file, errors.Join(err, <-finished)
	}
	snapshot, sourceErr := source.Fetch(ctx, sourceReader, repository, resolved.SHA)
	if sourceErr != nil || !snapshot.Complete || len(snapshot.Files) == 0 || snapshot.CommitSHA != resolved.SHA {
		t.Fatalf("private pinned source snapshot: %v", sourceErr)
	}
	reconcile := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReconcileChanges, Changes: &privateconnector.ChangesArgs{Repository: repository}}
	go func() { finished <- client.RunOnce(ctx) }()
	reconciled, reconcileErr := reader.Read(ctx, org, connection.ID, reconcile, func(context.Context, pgx.Tx, connections.Connection) error { return nil })
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if reconcileErr != nil || reconciled.Changes == nil || !reconciled.Changes.Complete || len(reconciled.Changes.Items) != 0 {
		t.Fatalf("actual private change reconciliation: %v", reconcileErr)
	}
	portfolio := inventory.New(db, identity, vault, reader, providers.DecodeWebhook)
	server.RegisterInventory(portfolio)
	syncResponse := identityRequest(server, "POST", "/api/v1/orgs/"+org+"/inventory-syncs", `{"connection_id":"`+connection.ID+`"}`, cookie, headers)
	var scan inventory.Job
	if syncResponse.Code != 202 || json.Unmarshal(syncResponse.Body.Bytes(), &scan) != nil {
		t.Fatalf("start inventory HTTP%d", syncResponse.Code)
	}
	drain := func(id string) inventory.Job {
		for i := 0; i < 20; i++ {
			job, err := portfolio.Job(ctx, session, org, id)
			if err != nil {
				t.Fatal(err)
			}
			if job.State == "complete" {
				return job
			}
			lease, err := portfolio.Claim(ctx, org, "private-fixture")
			if err != nil || lease == nil {
				t.Fatalf("inventory claim: %v", err)
			}
			if lease.Kind != "import" {
				go func() { finished <- client.RunOnce(ctx) }()
			}
			stepErr := portfolio.Step(ctx, *lease)
			if lease.Kind != "import" {
				if err = <-finished; err != nil {
					t.Fatal(err)
				}
			}
			if stepErr != nil {
				t.Fatalf("actual inventory step: %v", stepErr)
			}
		}
		t.Fatal("private inventory did not converge")
		return inventory.Job{}
	}
	scan = drain(scan.ID)
	candidates, candidateErr := portfolio.Candidates(ctx, session, org, scan.ID, 200, "")
	found := false
	for _, candidate := range candidates.Items {
		if candidate.NativeID == repository.NativeID {
			found = true
		}
	}
	if candidateErr != nil || !found {
		t.Fatalf("disposable native repository missing from scan: %v", candidateErr)
	}
	headers["If-Match"] = strconv.Quote(strconv.FormatInt(scan.Version, 10))
	importResponse := identityRequest(server, "POST", "/api/v1/orgs/"+org+"/inventory-syncs/"+scan.ID+"/import", `{"native_ids":["`+repository.NativeID+`"]}`, cookie, headers)
	var imported inventory.Job
	if importResponse.Code != 202 || json.Unmarshal(importResponse.Body.Bytes(), &imported) != nil {
		t.Fatalf("import HTTP%d", importResponse.Code)
	}
	imported = drain(imported.ID)
	if imported.Processed != 1 {
		t.Fatalf("imported %d repositories", imported.Processed)
	}
	if err = portfolio.Maintain(ctx, org); err != nil {
		t.Fatal(err)
	}
	lease, claimErr := portfolio.Claim(ctx, org, "private-fixture")
	if claimErr != nil || lease == nil || lease.Kind != "refresh" {
		t.Fatalf("private refresh claim %v", claimErr)
	}
	go func() { finished <- client.RunOnce(ctx) }()
	stepErr := portfolio.Step(ctx, *lease)
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if stepErr != nil {
		t.Fatal(stepErr)
	}
	drain(lease.ID)
	detail, detailErr := portfolio.Repository(ctx, session, org, lease.RepositoryID)
	if detailErr != nil || detail.SyncState != "fresh" || detail.ChangesObservedAt == nil || detail.NativeID != repository.NativeID {
		t.Fatalf("private portfolio freshness: %v", detailErr)
	}
	changes, changesErr := portfolio.Changes(ctx, session, org, lease.RepositoryID, 100, "")
	if changesErr != nil || changes.SnapshotState != "fresh" {
		t.Fatalf("private change snapshot freshness %s: %v", changes.SnapshotState, changesErr)
	}
	discoveries := discovery.New(db, identity, reader)
	server.RegisterDiscovery(discoveries)
	scanResponse := identityRequest(server, "POST", "/api/v1/orgs/"+org+"/repositories/"+detail.ID+"/discovery", "", cookie, headers)
	if scanResponse.Code != 202 {
		t.Fatalf("private discovery queue HTTP%d: %s", scanResponse.Code, scanResponse.Body.String())
	}
	discoveryContext, stopDiscovery := context.WithCancel(ctx)
	connectorDone := make(chan error, 1)
	go func() {
		for discoveryContext.Err() == nil {
			if err := client.RunOnce(discoveryContext); err != nil && !errors.Is(err, privateconnector.ErrUnavailable) && !errors.Is(err, privateconnector.ErrConflict) {
				connectorDone <- err
				return
			}
		}
		connectorDone <- discoveryContext.Err()
	}()
	worked, discoveryErr := discoveries.RunOrganisationOnce(discoveryContext, org)
	stopDiscovery()
	<-connectorDone
	discoveryState, stateErr := discoveries.ScanStatus(ctx, session, org, detail.ID)
	if discoveryErr != nil || stateErr != nil || !worked || discoveryState.State != "complete" {
		t.Fatalf("actual private discovery: worked=%v state=%s error=%v state_error=%v", worked, discoveryState.State, discoveryErr, stateErr)
	}
	findings, findingsErr := discoveries.List(ctx, session, org, 20, "", discovery.Filter{RepositoryID: detail.ID})
	if findingsErr != nil || len(findings.Items) != 1 || findings.Items[0].Category != "renovate_onboarding" || !findings.Items[0].Evidence.Complete {
		t.Fatalf("canonical private discovery findings: %d %v", len(findings.Items), findingsErr)
	}
	read.ID = domain.NewID()
	go func() { finished <- client.RunOnce(ctx) }()
	_, readErr = reader.Read(ctx, org, connection.ID, read, func(context.Context, pgx.Tx, connections.Connection) error { return auth.ErrForbidden })
	if !errors.Is(readErr, auth.ErrForbidden) {
		t.Fatalf("denied read authorization: %v", readErr)
	}
	if err = <-finished; !errors.Is(err, privateconnector.ErrUnavailable) {
		t.Fatalf("denied grant reached private executor: %v", err)
	}
	if os.Getenv("REFORGE_LOCAL_OLLAMA_TEST") == "1" {
		modelConnection, createErr := svc.Create(ctx, session, org, connections.CreateRequest{Kind: "model", Provider: "compatible", Name: "Private local Ollama", Endpoint: "http://127.0.0.1:55435", Settings: connections.Settings{AuthKind: "api_key", BillingRoute: "direct_api", Model: "qwen3:0.6b", Profile: "ollama"}, PrivateRoute: &connections.Route{RunnerID: supervisor.Runner.ID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}}, "test")
		if createErr != nil {
			t.Fatal(createErr)
		}
		go func() { finished <- client.RunOnce(ctx) }()
		checked, probeErr := svc.Test(ctx, session, org, modelConnection.ID, modelConnection.Version, "test")
		if err = <-finished; err != nil {
			t.Fatal(err)
		}
		if probeErr != nil || checked.State != "healthy" || len(checked.Capabilities) == 0 {
			t.Fatalf("private model metadata probe: state %s error %v", checked.State, probeErr)
		}
	}
	if _, err = runners.Claim(ctx, supervisor.Token); err == nil {
		t.Fatal("empty onboarding pool claimed a repair")
	}
	if _, err = svc.Revoke(ctx, session, org, connection.ID, verified.Version, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Test(ctx, session, org, connection.ID, verified.Version, "test"); !errors.Is(err, connections.ErrRevoked) && !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("revoked connection not blocked: %v", err)
	}
}

func localPrivateRepository(t *testing.T) forge.RepoRef {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(os.Getenv("REFORGE_TEST_ROOT"), ".local/gitea/reforge-admin.token"))
	if err != nil {
		t.Fatal("local fixture setup credential missing")
	}
	secret := strings.TrimSpace(string(raw))
	clear(raw)
	client := &http.Client{Timeout: 5 * time.Second}
	name := "private-read-" + domain.NewID()
	body, _ := json.Marshal(map[string]any{"name": name, "auto_init": false, "private": true, "default_branch": "main"})
	request, _ := http.NewRequest("POST", "http://127.0.0.1:53000/api/v1/admin/users/reforge-bot/repos", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "token "+secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("local disposable repository creation failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("disposable repository status %d", response.StatusCode)
	}
	var repo struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if json.NewDecoder(response.Body).Decode(&repo) != nil || repo.ID < 1 || !strings.HasSuffix(repo.FullName, "/"+name) {
		t.Fatal("disposable repository identity missing")
	}
	t.Cleanup(func() {
		request, _ := http.NewRequest("DELETE", "http://127.0.0.1:53000/api/v1/repos/"+repo.FullName, nil)
		request.Header.Set("Authorization", "token "+secret)
		response, err := client.Do(request)
		if err != nil {
			t.Error("disposable repository cleanup failed")
			return
		}
		response.Body.Close()
		if response.StatusCode != 204 {
			t.Errorf("disposable repository cleanup status %d", response.StatusCode)
		}
	})
	seed := strings.NewReader(`{"branch":"main","message":"Initialize disposable fixture","content":"Zml4dHVyZQo="}`)
	initial, _ := http.NewRequest("POST", "http://127.0.0.1:53000/api/v1/repos/"+repo.FullName+"/contents/.reforge-fixture", seed)
	initial.Header.Set("Authorization", "token "+secret)
	initial.Header.Set("Content-Type", "application/json")
	seeded, err := client.Do(initial)
	if err != nil {
		t.Fatal("disposable fixture initialization failed")
	}
	seeded.Body.Close()
	if seeded.StatusCode != http.StatusCreated {
		t.Fatalf("fixture initialization HTTP%d", seeded.StatusCode)
	}
	return forge.RepoRef{NativeID: strconv.FormatInt(repo.ID, 10), FullName: repo.FullName}
}
