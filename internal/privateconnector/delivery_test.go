package privateconnector

import (
	"reforge/internal/domain"
	"reforge/internal/forge"
	"strings"
	"testing"
)

func TestDeliveryOperationCannotChangeReadOrMutationAuthority(t *testing.T) {
	id := domain.NewID()
	request := forge.PipelineRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "acme/app"}, WorkflowID: "17", WorkflowPath: ".github/workflows/deploy.yml", WorkflowSHA: strings.Repeat("a", 40), SourceSHA: strings.Repeat("b", 40), ConfigSHA256: strings.Repeat("c", 64), Ref: "refs/heads/main", ArtifactDigest: "sha256:" + strings.Repeat("d", 64), Environment: "staging", CorrelationID: id, RulesHash: strings.Repeat("e", 64)}
	for _, kind := range []Kind{ForgePipelineInspect, ForgePipelineObserve, ForgePipelineTrigger, ForgePipelineRecover, ForgePipelineCancel} {
		t.Run(string(kind), func(t *testing.T) {
			in := request
			in.ObserveOnly = kind == ForgePipelineInspect || kind == ForgePipelineObserve
			if kind == ForgePipelineCancel {
				in.RunID = "71"
			}
			operation := Operation{ID: id, Kind: kind, Pipeline: &in}
			if err := operation.Validate(); err != nil {
				t.Fatal(err)
			}
			if operation.Mutation() == in.ObserveOnly {
				t.Fatal("read/write authority classification changed")
			}
			in.ObserveOnly = !in.ObserveOnly
			if operation.Validate() == nil {
				t.Fatal("read/write intent flip accepted")
			}
			in.ObserveOnly = !in.ObserveOnly
			operation.Delivery = &DeliveryArgs{Repository: in.Repository}
			if operation.Validate() == nil {
				t.Fatal("second payload accepted")
			}
			operation.Delivery = nil
			in.Inputs = map[string]string{"Reforge_environment": "production"}
			if operation.Validate() == nil {
				t.Fatal("native reserved identity override accepted")
			}
			in.Inputs = nil
			if kind == ForgePipelineTrigger || kind == ForgePipelineRecover {
				operation.ID = domain.NewID()
				if operation.Validate() == nil {
					t.Fatal("unbound mutation correlation accepted")
				}
			}
		})
	}
}
