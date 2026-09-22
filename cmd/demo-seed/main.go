package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const demoOrg = "00000000-0000-4000-8000-0000000000de"

type repository struct {
	id, native, name, branch, url string
	fresh                         bool
}

func main() {
	url := os.Getenv("REFORGE_DEMO_DATABASE_URL")
	if url == "" {
		url = os.Getenv("REFORGE_MIGRATION_DATABASE_URL")
	}
	if url == "" {
		fmt.Fprintln(os.Stderr, "set REFORGE_DEMO_DATABASE_URL to an owner connection that can bypass row-level security")
		os.Exit(1)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	var owner string
	if err := conn.QueryRow(ctx, `SELECT id::text FROM users WHERE email='owner@localhost' ORDER BY created_at LIMIT 1`).Scan(&owner); err != nil {
		fmt.Fprintln(os.Stderr, "no fixture owner user found; sign in once before seeding:", err)
		os.Exit(1)
	}

	repositories := []repository{
		{id: demoOrg[:len(demoOrg)-4] + "1001", native: "1001", name: "payments-api", branch: "main", url: "https://gitea.demo.internal/demo/payments-api", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1002", native: "1002", name: "checkout-web", branch: "main", url: "https://gitea.demo.internal/demo/checkout-web", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1003", native: "1003", name: "ledger-service", branch: "main", url: "https://gitea.demo.internal/demo/ledger-service", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1004", native: "1004", name: "notifications", branch: "main", url: "https://gitea.demo.internal/demo/notifications", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1005", native: "1005", name: "mobile-app", branch: "main", url: "https://gitea.demo.internal/demo/mobile-app", fresh: false},
		{id: demoOrg[:len(demoOrg)-4] + "1006", native: "1006", name: "data-pipeline", branch: "main", url: "https://gitea.demo.internal/demo/data-pipeline", fresh: true},
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('reforge.org_id',$1,true)`, demoOrg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exec := func(sql string, args ...any) {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			fmt.Fprintln(os.Stderr, "seed failed:", err)
			os.Exit(1)
		}
	}

	exec(`INSERT INTO organisations(id,name) VALUES($1,'Demo Operations') ON CONFLICT(id) DO UPDATE SET name=excluded.name`, demoOrg)
	exec(`INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true) ON CONFLICT(org_id,user_id) DO UPDATE SET role='owner',all_repositories=true`, demoOrg, owner)

	forge, model, agent, delivery := demoOrg[:len(demoOrg)-4]+"2001", demoOrg[:len(demoOrg)-4]+"2002", demoOrg[:len(demoOrg)-4]+"2003", demoOrg[:len(demoOrg)-4]+"2004"
	exec(`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason,server_version,verified_at) VALUES($1,$2,'forge','gitea','Gitea (demo)','https://gitea.demo.internal','{"auth_kind":"token","billing_route":"forge","namespace":"demo"}','healthy','Capability probe passed','1.27.3',now()) ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, forge)
	exec(`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason,server_version,verified_at) VALUES($1,$2,'model','openai','OpenAI (demo)','https://api.openai.com','{"auth_kind":"api_key","billing_route":"direct_api","model":"gpt-5"}','healthy','Capability probe passed','2026-09',now()) ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, model)
	exec(`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason,server_version) VALUES($1,$2,'agent','codex','Codex runtime (demo)','https://runtime.demo.internal','{"auth_kind":"official_runtime","billing_route":"subscription","model":"gpt-5","runtime_version":"0.150.1","namespace":"demo-workspace"}','disabled','Connect a documented official runtime and verify account entitlement, topology and budget controls','0.150.1') ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, agent)
	exec(`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason,server_version,verified_at) VALUES($1,$2,'delivery','github','GitHub Actions (demo)','https://api.github.com','{"auth_kind":"token","billing_route":"forge"}','healthy','Capability probe passed','2026-09',now()) ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, delivery)

	for _, r := range repositories {
		synced := time.Now().Add(-2 * time.Minute)
		if !r.fresh {
			synced = time.Now().Add(-26 * time.Hour)
		}
		exec(`INSERT INTO repositories(org_id,id,connection_id,native_id,name,url,default_branch,provider,accessible,last_synced_at) VALUES($1,$2,$3,$4,$5,$6,$7,'gitea',true,$8) ON CONFLICT(org_id,id) DO UPDATE SET name=excluded.name,last_synced_at=excluded.last_synced_at`, demoOrg, r.id, forge, r.native, r.name, r.url, r.branch, synced)
	}

	linuxPool, gpuPool := demoOrg[:len(demoOrg)-4]+"3001", demoOrg[:len(demoOrg)-4]+"3002"
	exec(`INSERT INTO runner_pools(org_id,id,name,state) VALUES($1,$2,'demo-linux','active') ON CONFLICT(org_id,id) DO UPDATE SET state='active'`, demoOrg, linuxPool)
	exec(`INSERT INTO runner_pools(org_id,id,name,state) VALUES($1,$2,'demo-gpu','draining') ON CONFLICT(org_id,id) DO UPDATE SET state='draining'`, demoOrg, gpuPool)
	for _, r := range repositories[:4] {
		exec(`INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, demoOrg, linuxPool, r.id)
	}
	exec(`INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, demoOrg, gpuPool, repositories[5].id)
	exec(`INSERT INTO runners(org_id,id,pool_id,name,state,credential_hash,credential_expires_at,last_seen_at) VALUES($1,$2,$3,'runner-linux-1','active','demo-credential',$4,now()-interval '12 seconds') ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, demoOrg[:len(demoOrg)-4]+"4001", linuxPool, time.Now().Add(30*24*time.Hour))
	exec(`INSERT INTO runners(org_id,id,pool_id,name,state,credential_hash,credential_expires_at,last_seen_at) VALUES($1,$2,$3,'runner-linux-2','active','demo-credential',$4,now()-interval '2 minutes') ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, demoOrg[:len(demoOrg)-4]+"4002", linuxPool, time.Now().Add(30*24*time.Hour))
	exec(`INSERT INTO runners(org_id,id,pool_id,name,state,credential_hash,credential_expires_at,last_seen_at) VALUES($1,$2,$3,'runner-gpu-1','active','demo-credential',$4,now()-interval '40 seconds') ON CONFLICT(org_id,id) DO NOTHING`, demoOrg, demoOrg[:len(demoOrg)-4]+"4003", gpuPool, time.Now().Add(30*24*time.Hour))

	findings := []struct {
		id, repository, title, category, severity string
	}{
		{"5001", repositories[0].id, "Upgrade axios to 1.7.9 (CVE-2025-58754)", "security_advisory", "high"},
		{"5002", repositories[1].id, "Lockfile drift after Node 22 upgrade", "dependency_update", "medium"},
		{"5003", repositories[2].id, "Failing unit test on target branch", "ci_failure", "high"},
		{"5004", repositories[3].id, "Renovate is not configured", "renovate_onboarding", "info"},
	}
	for _, f := range findings {
		evidence := map[string]any{
			"provenance": "gitea", "connection_id": forge, "connection_version": 1, "config_version": 1,
			"head_sha": strings.Repeat("a", 40), "target_sha": strings.Repeat("b", 40), "target_branch": "main",
			"checks": []any{}, "dependencies": []any{map[string]string{"ecosystem": "npm", "manifest": "package.json", "name": "axios", "from": "1.6.0", "to": "1.7.9"}},
			"ownership": "bot-owned", "bot": "renovate", "complete": true, "blockers": []any{}, "merge_blockers": []any{},
		}
		raw, _ := json.Marshal(evidence)
		exec(`INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest,state,reason,version,first_seen,last_seen) VALUES($1,$2,$3,$4,'repository',$5,$6,$7,$8,$9,$10,'open','Demo finding',1,now()-interval '2 days',now()-interval '1 hour') ON CONFLICT(org_id,id) DO NOTHING`,
			demoOrg, demoOrg[:len(demoOrg)-4]+f.id, f.repository, strings.Repeat("f", 64), "demo-"+f.id, f.category, f.severity, f.title, raw, strings.Repeat("e", 64))
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("demo organisation seeded: %s (%s)\n", demoOrg, "Demo Operations")
}
