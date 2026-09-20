package gitlab

import (
	"context"
	"net/http"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"strings"
	"testing"
)

func TestTrainReleaseRejectsFreshIdentityDriftBeforePlay(t *testing.T) {
	for _, mode := range []string{"head", "publisher", "protected_environment", "configuration"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newTrainGateFixture()
			mutate := false
			client := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if mutate && strings.HasSuffix(request.URL.Path, "/protected_environments") && mode == "protected_environment" {
					return jsonResponse(200, []any{}), nil
				}
				if mutate && strings.HasSuffix(request.URL.Path, "/.gitlab-ci.yml/raw") && mode == "configuration" {
					return jsonResponse(200, map[string]any{}), nil
				}
				return fixture.Do(request)
			})
			provider, err := New(forge.Config{BaseURL: "https://gitlab.example", Token: "token", Client: client})
			if err != nil {
				t.Fatal(err)
			}
			provider = provider.WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil }).WithCheckPublishers(map[string]string{trainGateName: "81"}).WithTrainGateAuthorizer(func(context.Context, forge.TrainGateRequest) error { return nil })
			gate, err := provider.ReadTrainGate(context.Background(), testRepo, "9")
			if err != nil {
				t.Fatal(err)
			}
			rules, err := provider.ReadEffectiveRules(context.Background(), testRepo, "main")
			if err != nil {
				t.Fatal(err)
			}
			mutate = true
			if mode == "head" {
				fixture.head = strings.Repeat("4", 40)
			}
			if mode == "publisher" {
				provider = provider.WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil }).WithCheckPublishers(map[string]string{trainGateName: "82"})
			}
			_, err = provider.ReleaseTrainGate(context.Background(), forge.TrainGateRequest{RulesHash: rules.Hash, Repository: testRepo, ChangeID: "9", Gate: gate, OperationID: domain.NewID()})
			if err == nil || fixture.plays != 0 {
				t.Fatalf("drift released train: %v plays=%d", err, fixture.plays)
			}
		})
	}
}
