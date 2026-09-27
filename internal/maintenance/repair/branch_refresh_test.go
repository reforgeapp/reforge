package repair

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type refreshDBConnector struct {
	change  forge.Change
	behind  int
	writes  int
	reads   int
	db      *store.Store
	org     string
	version int64
}

func (c *refreshDBConnector) SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{}
}

func (c *refreshDBConnector) Read(ctx context.Context, org, connection string, op privateconnector.Operation, check func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	c.reads++
	var result privateconnector.Result
	err := c.db.Tenant(ctx, c.org, "", func(tx pgx.Tx) error {
		if err := check(ctx, tx, connections.Connection{ID: connection, Version: c.version, State: "healthy"}); err != nil {
			return err
		}
		switch op.Kind {
		case privateconnector.ForgeReadChange:
			result.Change = &c.change
		case privateconnector.ForgeResolveRef:
			result.SHA = strings.Repeat("b", 40)
		case privateconnector.ForgeBehind:
			result.Behind = &c.behind
		default:
			return privateconnector.ErrUnsupported
		}
		return nil
	})
	return result, err
}

func (c *refreshDBConnector) Write(ctx context.Context, org, connection string, op privateconnector.Operation, prepare func(context.Context, pgx.Tx, connections.Connection) (string, error), commit func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	var result privateconnector.Result
	err := c.db.Tenant(ctx, c.org, "", func(tx pgx.Tx) error {
		connection := connections.Connection{ID: connection, Version: c.version, State: "healthy"}
		if _, err := prepare(ctx, tx, connection); err != nil {
			return err
		}
		if err := commit(ctx, tx, connection); err != nil {
			return err
		}
		c.writes++
		result.OperationID = op.ID
		return nil
	})
	return result, err
}

func TestRefreshFixRequiresOwnedIdleStaleBranch(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(db, identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}
	org, repo, connection, taskID, findingID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	branch := "reforge/repair/" + taskID
	change := forge.Change{ID: "42", Repository: forge.RepoRef{NativeID: "native-repo", FullName: "team/repo"}, HeadRepository: forge.RepoRef{NativeID: "native-repo", FullName: "team/repo"}, TargetRepository: forge.RepoRef{NativeID: "native-repo", FullName: "team/repo"}, HeadSHA: strings.Repeat("a", 40), TargetSHA: strings.Repeat("9", 40), HeadBranch: branch, TargetBranch: "main", State: "open"}
	connector := &refreshDBConnector{db: db, org: org, version: 1, change: change, behind: 2}
	if err = db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'repair-refresh-test',$1::text,'Owner','owner@example.test')`, session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, session.ID, session.User.ID, domain.NewID(), domain.NewID())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Repair refresh')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, session.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','gitea','test','http://127.0.0.1:53000','{}','healthy')`, org, connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name,connection_id) VALUES($1,$2,$3,'team/repo',$4)`, org, repo, change.Repository.NativeID, connection); err != nil {
			return err
		}
		policyBody, _ := json.Marshal(policy.Policy{Schema: "maintenance/v1"})
		if _, err := tx.Exec(ctx, `INSERT INTO policy_versions(org_id,id,scope_kind,scope_id,document,policy_hash,actor_id,reason) VALUES($1,$1,'organisation',$1,$2,$3,$4,'test')`, org, policyBody, strings.Repeat("c", 64), session.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_bindings(org_id,scope_kind,scope_id,policy_version_id,version,simulation_hash) VALUES($1,'organisation',$1,$1,1,$2)`, org, strings.Repeat("d", 64)); err != nil {
			return err
		}
		evidence, _ := json.Marshal(map[string]any{})
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','42','dependency','medium','test',$5,$6)`, org, findingID, repo, strings.Repeat("e", 64), evidence, strings.Repeat("f", 64)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'go','1','main',$6,$6,'completed',1,$7)`, org, taskID, repo, domain.NewID(), taskID, strings.Repeat("1", 64), session.User.ID); err != nil {
			return err
		}
		contextBody, _ := json.Marshal(ExecutionContext{ConnectionID: connection, ConnectionVersion: 1, Repository: change.Repository})
		changeBody, _ := json.Marshal(change)
		_, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,branch,candidate_sha,native_change,candidate_artifacts) VALUES($1,$2,$3,$4,$5,1,$6,$7,'published',$8,$9,$10,'[]')`, org, taskID, repo, findingID, session.User.ID, strings.Repeat("2", 64), contextBody, branch, change.HeadSHA, changeBody)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	jobs := workflow.New(db, identity, nil)
	service := New(db, identity, discovery.New(db, identity, connector), jobs, nil, policies, nil, connections.New(db, identity, nil, true), nil, connector, nil)

	assert := func(name string, want bool, wantReads, wantWrites int) {
		t.Helper()
		got, err := service.RefreshFix(ctx, session, org, taskID)
		if err != nil || got != want || connector.reads != wantReads || connector.writes != wantWrites {
			t.Fatalf("%s: refresh=%t err=%v reads=%d writes=%d", name, got, err, connector.reads, connector.writes)
		}
	}
	connector.behind = 0
	assert("current branch", false, 3, 0)
	connector.reads = 0
	connector.change.MergeStatus = "dirty"
	if waiting, conflictErr := service.RefreshFix(ctx, session, org, taskID); waiting || conflictErr != ErrBranchConflict || connector.reads != 1 || connector.writes != 0 {
		t.Fatalf("conflicting branch dispatched refresh: waiting=%v err=%v reads=%d writes=%d", waiting, conflictErr, connector.reads, connector.writes)
	}
	connector.change.MergeStatus = ""
	connector.reads, connector.behind = 0, 2
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET branch='external/branch',native_change=jsonb_set(native_change,'{head_branch}','"external/branch"') WHERE org_id=$1 AND task_id=$2`, org, taskID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assert("foreign branch", false, 0, 0)
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET branch=$3,native_change=jsonb_set(native_change,'{head_branch}',to_jsonb($3::text)) WHERE org_id=$1 AND task_id=$2`, org, taskID, branch)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET native_change=jsonb_set(native_change,'{state}','"closed"') WHERE org_id=$1 AND task_id=$2`, org, taskID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assert("closed change", false, 0, 0)
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET native_change=jsonb_set(native_change,'{state}','"open"') WHERE org_id=$1 AND task_id=$2`, org, taskID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	connector.reads = 0
	assertRejected := func(name string, want error) {
		t.Helper()
		beforeReads, beforeWrites := connector.reads, connector.writes
		if _, err := service.RefreshFix(ctx, session, org, taskID); err == nil || err != want || connector.reads != beforeReads || connector.writes != beforeWrites {
			t.Fatalf("%s: err=%v want=%v reads=%d/%d writes=%d/%d", name, err, want, beforeReads, connector.reads, beforeWrites, connector.writes)
		}
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE organisations SET paused=true WHERE id=$1`, org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertRejected("paused policy", workflow.ErrPolicy)
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE organisations SET paused=false WHERE id=$1`, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE repositories SET accessible=false WHERE org_id=$1 AND id=$2`, org, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertRejected("inaccessible repository", workflow.ErrPolicy)
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repositories SET accessible=true WHERE org_id=$1 AND id=$2`, org, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	connector.reads = 0
	busyTask, busyFinding := domain.NewID(), domain.NewID()
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','busy','dependency','medium','busy','{}',$5)`, org, busyFinding, repo, strings.Repeat("6", 64), strings.Repeat("7", 64)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'go','1','main',$6,$6,'planning',1,$7)`, org, busyTask, repo, domain.NewID(), domain.NewID(), strings.Repeat("3", 64), session.User.ID); err != nil {
			return err
		}
		busyContext, _ := json.Marshal(ExecutionContext{FollowUpBranch: branch})
		_, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,branch) VALUES($1,$2,$3,$4,$5,1,$6,$7,'queued','reforge/repair/busy')`, org, busyTask, repo, busyFinding, session.User.ID, strings.Repeat("8", 64), busyContext)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assert("active follow-up workflow", true, 0, 0)
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state='completed' WHERE org_id=$1 AND id=$2`, org, busyTask)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	connector.reads = 0
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET version=2 WHERE org_id=$1 AND id=$2`, org, connection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	connector.version = 2
	connector.behind = 2
	assert("stale owned branch", true, 3, 1)
}
