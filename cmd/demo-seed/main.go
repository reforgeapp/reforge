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
	if os.Getenv("REFORGE_MODE") != "development" || os.Getenv("REFORGE_DEMO_SEED") != "1" {
		fmt.Fprintln(os.Stderr, "set REFORGE_MODE=development and REFORGE_DEMO_SEED=1 to seed development-only demo records")
		os.Exit(1)
	}
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
		{id: demoOrg[:len(demoOrg)-4] + "1001", native: "1001", name: "payments-api", branch: "main", url: "https://gitea.demo.invalid/demo/payments-api", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1002", native: "1002", name: "checkout-web", branch: "main", url: "https://gitea.demo.invalid/demo/checkout-web", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1003", native: "1003", name: "ledger-service", branch: "main", url: "https://gitea.demo.invalid/demo/ledger-service", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1004", native: "1004", name: "notifications", branch: "main", url: "https://gitea.demo.invalid/demo/notifications", fresh: true},
		{id: demoOrg[:len(demoOrg)-4] + "1005", native: "1005", name: "mobile-app", branch: "main", url: "https://gitea.demo.invalid/demo/mobile-app", fresh: false},
		{id: demoOrg[:len(demoOrg)-4] + "1006", native: "1006", name: "data-pipeline", branch: "main", url: "https://gitea.demo.invalid/demo/data-pipeline", fresh: true},
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
	seedConnection := func(id, kind, provider, name, endpoint, settings string) {
		exec(`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason,server_version,verified_at) VALUES($1,$2,$3,$4,$5,$6,$7,'unverified','Development demo record; capability has not been probed','',NULL) ON CONFLICT(org_id,id) DO UPDATE SET name=excluded.name,endpoint=excluded.endpoint,settings=excluded.settings,state=excluded.state,reason=excluded.reason,server_version=excluded.server_version,verified_at=NULL`, demoOrg, id, kind, provider, name, endpoint, settings)
	}
	seedConnection(forge, "forge", "gitea", "Gitea (demo, unverified)", "https://gitea.demo.invalid", `{"auth_kind":"token","billing_route":"forge","namespace":"demo"}`)
	seedConnection(model, "model", "openai", "OpenAI (demo, unverified)", "https://api.openai.demo.invalid", `{"auth_kind":"api_key","billing_route":"direct_api"}`)
	seedConnection(agent, "agent", "codex", "Codex runtime (demo, unverified)", "https://runtime.demo.invalid", `{"auth_kind":"official_runtime","billing_route":"subscription"}`)
	seedConnection(delivery, "delivery", "github", "GitHub Actions (demo, unverified)", "https://github.demo.invalid", `{"auth_kind":"token","billing_route":"forge"}`)
	for _, r := range repositories {
		synced := time.Now().Add(-2 * time.Minute)
		if !r.fresh {
			synced = time.Now().Add(-26 * time.Hour)
		}
		exec(`INSERT INTO repositories(org_id,id,connection_id,native_id,name,url,default_branch,provider,accessible,last_synced_at) VALUES($1,$2,$3,$4,$5,$6,$7,'gitea',true,$8) ON CONFLICT(org_id,id) DO UPDATE SET name=excluded.name,url=excluded.url,last_synced_at=excluded.last_synced_at`, demoOrg, r.id, forge, r.native, r.name, r.url, r.branch, synced)
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
		{"5001", repositories[0].id, "Dependency advisory example (demo)", "security_advisory", "high"},
		{"5002", repositories[1].id, "Lockfile drift after Node 22 upgrade", "dependency_update", "medium"},
		{"5003", repositories[2].id, "Failing unit test on target branch", "ci_failure", "high"},
		{"5004", repositories[3].id, "Renovate is not configured", "renovate_onboarding", "info"},
	}
	for _, f := range findings {
		evidence := map[string]any{
			"provenance": "gitea", "connection_id": forge, "connection_version": 1, "config_version": 1,
			"head_sha": strings.Repeat("a", 40), "target_sha": strings.Repeat("b", 40), "target_branch": "main",
			"checks": []any{}, "dependencies": []any{},
			"ownership": "bot-owned", "bot": "renovate", "complete": true, "blockers": []any{}, "merge_blockers": []any{},
		}
		raw, _ := json.Marshal(evidence)
		exec(`INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest,state,reason,version,first_seen,last_seen) VALUES($1,$2,$3,$4,'repository',$5,$6,$7,$8,$9,$10,'open','Demo finding',1,now()-interval '2 days',now()-interval '1 hour') ON CONFLICT(org_id,id) DO UPDATE SET title=excluded.title,evidence=excluded.evidence,evidence_digest=excluded.evidence_digest,category=excluded.category,severity=excluded.severity,reason=excluded.reason,last_seen=excluded.last_seen`,
			demoOrg, demoOrg[:len(demoOrg)-4]+f.id, f.repository, strings.Repeat("f", 64), "demo-"+f.id, f.category, f.severity, f.title, raw, strings.Repeat("e", 64))
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("demo organisation seeded: %s (%s)\n", demoOrg, "Demo Operations")
}
