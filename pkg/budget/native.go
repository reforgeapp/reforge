package budget

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func ConfigureNativeTx(ctx context.Context, tx pgx.Tx, org, connection, model, evidence string) error {
	route := Route{ConnectionID: connection, Model: model, Name: "default", Mode: "quota", MaxInputTokens: 1<<20 + 4096, MaxOutputTokens: 8192, MaxMilliseconds: 600000, MaxRequests: 1, Qualified: true, QualificationRef: evidence, Version: 1}
	raw, err := json.Marshal(route)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,$3,'default',$4,1) ON CONFLICT(org_id,connection_id,model,name) DO UPDATE SET config=jsonb_set(excluded.config,'{version}',to_jsonb(budget_routes.version+1)),version=budget_routes.version+1`, org, connection, model, raw); err != nil {
		return err
	}
	caps := `{"tokens":20000000,"milliseconds":21600000,"requests":1000,"concurrency":1}`
	if _, err = tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,version,caps) VALUES($1,'connection',$2,'daily',date_trunc('day',now()),date_trunc('day',now())+interval '1 day',1,$3) ON CONFLICT(org_id,scope_kind,scope_id) DO NOTHING`, org, connection, []byte(caps)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,version,caps) VALUES($1,'organisation',$1,'monthly',date_trunc('month',now()),date_trunc('month',now())+interval '1 month',1,'{"micro_usd":0,"tokens":20000000,"milliseconds":21600000,"requests":1000,"concurrency":2}'::jsonb) ON CONFLICT(org_id,scope_kind,scope_id) DO NOTHING`, org); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE budget_limits SET caps=jsonb_build_object('tokens',20000000,'milliseconds',21600000,'requests',1000,'concurrency',2)||jsonb_strip_nulls(caps),version=version+1 WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1 AND (caps->>'tokens' IS NULL OR caps->>'milliseconds' IS NULL OR caps->>'requests' IS NULL OR caps->>'concurrency' IS NULL)`, org)
	return err
}

func NativeEligible(r Route) bool {
	return validRoute(r) && r.Mode == "quota" && !r.Paused && r.Qualified && r.QualificationRef != "" && r.MaxRequests == 1
}

func AutomaticConnectionSQL(connection, route string) string {
	return "(" + connection + `.kind='model' AND ` + AutomaticRouteSQL(route) + " OR " + connection + `.kind='agent' AND ` + connection + `.provider IN ('codex','claude_code') AND ` + connection + `.settings->>'managed'='native_runtime' AND ` + connection + `.settings->>'auth_kind'='official_runtime' AND ` + connection + `.settings->>'billing_route'='subscription' AND coalesce(` + connection + `.settings->>'namespace','')<>'' AND ` + route + `.config @? '$ ? (@.mode == "quota" && @.qualified == true && @.qualification_ref != "" && @.paused == false && @.max_input_tokens > 0 && @.max_output_tokens > 0 && @.max_milliseconds > 0 && @.max_requests == 1 && @.input_micro_usd_per_million == 0 && @.output_micro_usd_per_million == 0 && @.request_micro_usd == 0)' AND ` + route + `.config->>'connection_id'=` + route + `.connection_id::text AND ` + route + `.config->>'model'=` + route + `.model AND ` + route + `.config->>'name'=` + route + `.name)`
}

func (s *Service) ModelHeadroomTx(ctx context.Context, tx pgx.Tx, org, connection string) error {
	if connection == "" {
		return s.HeadroomTx(ctx, tx, org)
	}
	var native bool
	if err := tx.QueryRow(ctx, `SELECT coalesce(kind='agent' AND settings->>'managed'='native_runtime',false) FROM connections WHERE org_id=$1 AND id=$2`, org, connection).Scan(&native); err != nil {
		return err
	}
	if !native {
		return s.HeadroomTx(ctx, tx, org)
	}
	return nativeHeadroomTx(ctx, tx, org, Scope{Kind: "organisation", ID: org}, true)
}

func (s *Service) RepositoryModelHeadroomTx(ctx context.Context, tx pgx.Tx, org, repository, connection string) error {
	var native bool
	if connection != "" {
		if err := tx.QueryRow(ctx, `SELECT coalesce(kind='agent' AND settings->>'managed'='native_runtime',false) FROM connections WHERE org_id=$1 AND id=$2`, org, connection).Scan(&native); err != nil {
			return err
		}
	}
	if native {
		return nativeHeadroomTx(ctx, tx, org, Scope{Kind: "repository", ID: repository}, false)
	}
	return s.RepositoryHeadroomTx(ctx, tx, org, repository)
}

func nativeHeadroomTx(ctx context.Context, tx pgx.Tx, org string, scope Scope, required bool) error {
	limit, err := loadLimit(ctx, tx, org, scope)
	if errors.Is(err, pgx.ErrNoRows) && !required {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnknown
	}
	if err != nil {
		return err
	}
	if limit.Paused {
		return ErrRevoked
	}
	if required && !quotaCaps(limit.Caps) {
		return ErrUnknown
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	start, err := periodStart(limit, now)
	if err != nil {
		return err
	}
	spent, err := loadSpend(ctx, tx, org, scope, start)
	if err != nil {
		return err
	}
	total, err := plus(limit.Held, spent)
	if err != nil {
		return err
	}
	for i, cap := range capValues(limit.Caps) {
		if i != 0 && cap != nil && values(total)[i] >= *cap {
			return ErrCapacity
		}
	}
	return nil
}
