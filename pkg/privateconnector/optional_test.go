package privateconnector

import (
	"context"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestUnsupportedOptionalForgeReadPreservesCapabilityReason(t *testing.T) {
	f := fixture(t)
	op := Operation{ID: domain.NewID(), Kind: ForgeIssues, Repository: &RepositoryArgs{Repository: forge.RepoRef{NativeID: "1", FullName: "owner/app"}}}
	spec := f.spec(op.ID)
	spec.Connection.Kind = "forge"
	grant := Grant{ID: domain.NewID(), RunnerVersion: 1, Target: f.target, Operation: op, AuthorityID: spec.AuthorityID, Connection: spec.Connection, TimeoutMS: 1000, ExpiresAt: time.Now().Add(time.Second), executionDeadline: time.Now().Add(time.Second), ResultCapability: "0123456789012345678901234567890123456789012"}
	result := (Executor{Target: f.target, Development: true}).Execute(context.Background(), grant)
	if result.Failure == nil || result.Failure.Code != "unsupported" || result.Failure.Uncertain {
		t.Fatalf("optional absence turned into scan failure: %+v", result.Failure)
	}
}
