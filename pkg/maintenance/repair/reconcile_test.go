package repair

import (
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

func TestStageRecoveryRequiresExactParentMarkerAndWholeTree(t *testing.T) {
	p, _ := testPlan(t)
	baseline := sandbox.Snapshot{CommitSHA: p.TargetSHA, Complete: true, Files: []guest.File{{Path: "value.js", Content: []byte("old")}, {Path: "value.test.js", Content: []byte("frozen")}}}
	baseline.ManifestSHA256, _ = sandbox.SnapshotDigest(baseline.Files)
	r := Run{Context: ExecutionContext{Plan: p}, Report: &Report{Patches: []sandbox.Patch{{Path: "value.js", Content: []byte("new")}}}}
	c := sandbox.Snapshot{CommitSHA: strings.Repeat("d", 40), Complete: true, Files: []guest.File{{Path: "value.js", Content: []byte("new")}, {Path: "value.test.js", Content: []byte("frozen")}}}
	c.ManifestSHA256, _ = sandbox.SnapshotDigest(c.Files)
	proof := forge.CommitProof{SHA: c.CommitSHA, Parents: []string{p.TargetSHA}, Message: "Repair\n\n[reforge-operation:operation]"}
	if !stageMatches(r, proof, "operation", baseline, c) {
		t.Fatal("exact native patch rejected")
	}
	for _, name := range []string{"parent", "marker", "extra", "test", "mode", "incomplete"} {
		t.Run(name, func(t *testing.T) {
			copy := c
			copy.Files = append([]guest.File(nil), c.Files...)
			bad := proof
			switch name {
			case "parent":
				bad.Parents = []string{strings.Repeat("e", 40)}
			case "marker":
				bad.Message = "different"
			case "extra":
				copy.Files = append(copy.Files, guest.File{Path: "other.js", Content: []byte("backdoor")})
			case "test":
				copy.Files[1].Content = []byte("disabled")
			case "mode":
				copy.Files[0].Executable = true
			case "incomplete":
				copy.Complete = false
			}
			copy.ManifestSHA256, _ = sandbox.SnapshotDigest(copy.Files)
			if stageMatches(r, bad, "operation", baseline, copy) {
				t.Fatal("unproven candidate accepted")
			}
		})
	}
}
