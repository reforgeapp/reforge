package discovery

import (
	"testing"

	"reforge/internal/maintenance/detectors"
)

func TestFingerprintCanonicalizesDependencyOrder(t *testing.T) {
	base := Observation{RepositoryID: "repo", Source: "forge_change", SourceID: "change", Category: "dependency_update", Evidence: Evidence{ConnectionID: "connection", TargetBranch: "main", Dependencies: []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "z", From: "1"}, {Ecosystem: "go", Manifest: "go.mod", Name: "a", From: "1"}}}}
	reordered := base
	reordered.Evidence.Dependencies = []detectors.Dependency{base.Evidence.Dependencies[1], base.Evidence.Dependencies[0]}
	if Fingerprint("org", base) != Fingerprint("org", reordered) {
		t.Fatal("dependency order changed fingerprint")
	}
}

func TestFingerprintIncludesDependencyGroupAndTargetBranch(t *testing.T) {
	base := Observation{RepositoryID: "repo", Source: "forge_change", SourceID: "change", Category: "dependency_update", Evidence: Evidence{ConnectionID: "connection", TargetBranch: "main", Dependencies: []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "a", From: "1"}}}}
	group := base
	group.Evidence.Dependencies = append([]detectors.Dependency{}, base.Evidence.Dependencies...)
	group.Evidence.Dependencies = append(group.Evidence.Dependencies, detectors.Dependency{Ecosystem: "go", Manifest: "go.mod", Name: "b", From: "1"})
	branch := base
	branch.Evidence.TargetBranch = "release"
	if Fingerprint("org", base) == Fingerprint("org", group) {
		t.Fatal("dependency group was ignored")
	}
	if Fingerprint("org", base) == Fingerprint("org", branch) {
		t.Fatal("target branch was ignored")
	}
}

func TestOverlapUsesDependencyIdentity(t *testing.T) {
	a := []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "a", From: "1"}}
	b := []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "a", From: "2"}}
	c := []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "b", From: "1"}}
	if !Overlap(a, b) || Overlap(a, c) {
		t.Fatal("partial dependency overlap calculation incorrect")
	}
}
