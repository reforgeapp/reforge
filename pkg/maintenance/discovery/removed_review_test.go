package discovery

import (
	"testing"

	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestRemovedReviewPathRequiresMatchingReviewEvidence(t *testing.T) {
	repo := forge.RepoRef{NativeID: "repo", FullName: "team/repo"}
	original := forge.SourceManifest{Repository: repo, CommitSHA: "old", Complete: true, Entries: []forge.SourceEntry{{Path: "src/old.go", Type: "blob"}}}
	current := forge.SourceManifest{Repository: repo, CommitSHA: "new", Complete: true}
	finding := Finding{Observation: Observation{Source: "repository_review", Evidence: Evidence{HeadSHA: "old", Review: &ReviewEvidence{Path: "src/old.go"}}}}
	if !removedReviewPath(finding, original, current) {
		t.Fatal("removed file from matching complete review was not recognized")
	}

	for _, tc := range []struct {
		name    string
		finding Finding
		before  forge.SourceManifest
		current forge.SourceManifest
	}{
		{"user report", Finding{Observation: Observation{Source: "user_report", Evidence: finding.Evidence}}, original, current},
		{"path absent from original", finding, forge.SourceManifest{Repository: repo, CommitSHA: "old", Complete: true}, current},
		{"file still exists", finding, original, forge.SourceManifest{Repository: repo, CommitSHA: "new", Complete: true, Entries: []forge.SourceEntry{{Path: "src/old.go", Type: "blob"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if removedReviewPath(tc.finding, tc.before, tc.current) {
				t.Fatal("unrelated or unproven removal was accepted")
			}
		})
	}
}
