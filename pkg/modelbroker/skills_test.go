package modelbroker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
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
