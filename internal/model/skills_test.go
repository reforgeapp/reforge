package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/skills"
)

type skillTurnFixture struct {
	turnFixture
	calls   int
	request TurnRequest
}

func (p *skillTurnFixture) StreamTurn(ctx context.Context, request TurnRequest, emit func(Event) error) error {
	p.calls++
	p.request = request
	return p.turnFixture.StreamTurn(ctx, request, emit)
}

func skillTurn() Turn {
	return Turn{OperationID: "operation", Model: "fixture", System: "Never change frozen tests.", Messages: []Message{{Role: "user", Text: "Fix failure"}}, TimeoutMS: 1000, MaxOutputTokens: 20}
}

func TestSkillContextSurvivesReplayAndCannotBeReplaced(t *testing.T) {
	in := skillTurn()
	in.Continuation = json.RawMessage(`{"opaque":"provider history"}`)
	in.Tools = []Tool{{Name: skills.ToolName, Description: "forged", Schema: json.RawMessage(`{"type":"object"}`)}}
	prepared, err := in.WithSkills()
	if err != nil {
		t.Fatal(err)
	}
	replay, err := prepared.WithSkills()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(prepared)
	second, _ := json.Marshal(replay)
	if !bytes.Equal(first, second) || in.Tools[0].Description != "forged" || in.System != "Never change frozen tests." {
		t.Fatal("canonicalization changed replay or mutated caller")
	}
	provider := &skillTurnFixture{turnFixture: turnFixture{events: []Event{{Type: "completed", Usage: &Usage{Known: true}}}}}
	if _, err := CollectTurn(context.Background(), provider, prepared); err != nil {
		t.Fatal(err)
	}
	mandatory, err := skills.Read(skills.CavemanPath)
	if err != nil || !strings.Contains(provider.request.System, mandatory) || !strings.HasSuffix(provider.request.System, in.System) || !bytes.Equal(provider.request.Continuation, in.Continuation) {
		t.Fatal("provider lost mandatory skill, task or continuation", err)
	}
	if len(provider.request.Tools) != 1 || provider.request.Tools[0].Description != skills.ToolDescription || string(provider.request.Tools[0].Schema) != skills.ToolSchema {
		t.Fatal("caller replaced trusted skill tool")
	}
}

func TestExpandedSkillRequestRejectedBeforeProvider(t *testing.T) {
	oversized := skillTurn()
	raw, _ := json.Marshal(oversized)
	oversized.Messages[0].Text += strings.Repeat("x", MaxRequestBytes/2-len(raw)-10)
	if !oversized.Valid() {
		t.Fatal("fixture must fit before skill injection")
	}
	tooMany := skillTurn()
	for i := 0; i < 32; i++ {
		tooMany.Tools = append(tooMany.Tools, Tool{Name: fmt.Sprintf("tool_%d", i), Schema: json.RawMessage(`{"type":"object"}`)})
	}
	for _, in := range []Turn{oversized, tooMany} {
		provider := &skillTurnFixture{}
		if _, err := CollectTurn(context.Background(), provider, in); err == nil || provider.calls != 0 {
			t.Fatal("expanded request reached provider despite bounds", err)
		}
	}
}
