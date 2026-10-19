package modelbroker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestSkillExpansionRejectedBeforeJobOrBudgetAccess(t *testing.T) {
	service := &Service{AuthorizeReservation: func(context.Context, pgx.Tx, workflow.Task, budget.Reservation) error {
		t.Fatal("oversized expanded request reached budget authority")
		return nil
	}}
	in := model.Turn{OperationID: domain.NewID(), Model: "fixture", Messages: []model.Message{{Role: "user", Text: "repair"}}, TimeoutMS: 1000, MaxOutputTokens: 20}
	raw, _ := json.Marshal(in)
	in.Messages[0].Text += strings.Repeat("x", model.MaxRequestBytes/2-len(raw)-10)
	if !in.Valid() {
		t.Fatal("raw fixture must fit")
	}
	if _, err := service.Turn(context.Background(), "unused", in); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("expanded request not rejected at broker boundary: %v", err)
	}
}

func TestExistingReplaysKnownProviderRejections(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "reforge_test" {
		t.Skip("test database must be reforge_test")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(context.Background(), `CREATE TEMP TABLE model_turns (org_id text, id text, state text, request_hash text, task_id text, attempt_id text, result_envelope bytea, failure text)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"billing", "quota", "rate_limit", "rate_limited", "uncertain", "auth"} {
		_, err = tx.Exec(context.Background(), `INSERT INTO model_turns (org_id,id,state,request_hash,task_id,attempt_id,failure) VALUES ('org',$1,'failed','hash','task','attempt',$2)`, failure, failure)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = tx.Exec(context.Background(), `INSERT INTO model_turns (org_id,id,state,request_hash,task_id,attempt_id,failure) VALUES ('org','conflict','failed','other','task','attempt','billing')`)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{}
	lease := workflow.Lease{OrgID: "org", TaskID: "task", AttemptID: "attempt"}
	for _, failure := range []string{"billing", "quota", "rate_limit", "rate_limited"} {
		_, err = service.existing(context.Background(), tx, lease, failure, "hash")
		var provider *domain.ProviderError
		if !errors.As(err, &provider) || provider.Kind != failure || errors.Is(err, ErrTurnFailed) {
			t.Fatalf("failure %s replayed as %v", failure, err)
		}
		if failure == "billing" && provider.RetryAfter != 15*time.Minute {
			t.Fatalf("billing replay delay=%v", provider.RetryAfter)
		}
	}
	for _, failure := range []string{"uncertain", "auth"} {
		if _, err = service.existing(context.Background(), tx, lease, failure, "hash"); !errors.Is(err, ErrTurnFailed) {
			t.Fatalf("failure %s replay behavior changed: %v", failure, err)
		}
	}
	if _, err = service.existing(context.Background(), tx, lease, "conflict", "hash"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("request conflict behavior changed: %v", err)
	}
}
