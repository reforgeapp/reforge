package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/maintenance/detectors"
)

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func canonical(in Observation) Observation {
	in.Evidence.Dependencies = append([]detectors.Dependency{}, in.Evidence.Dependencies...)
	sort.Slice(in.Evidence.Dependencies, func(i, j int) bool { return digest(in.Evidence.Dependencies[i]) < digest(in.Evidence.Dependencies[j]) })
	in.Evidence.Checks = append([]forge.Check{}, in.Evidence.Checks...)
	sort.Slice(in.Evidence.Checks, func(i, j int) bool { return in.Evidence.Checks[i].ID < in.Evidence.Checks[j].ID })
	in.Evidence.Blockers = append([]string{}, in.Evidence.Blockers...)
	sort.Strings(in.Evidence.Blockers)
	in.Evidence.MergeBlockers = append([]string{}, in.Evidence.MergeBlockers...)
	sort.Strings(in.Evidence.MergeBlockers)
	in.Evidence.TrackedFiles = append([]forge.SourceEntry{}, in.Evidence.TrackedFiles...)
	sort.Slice(in.Evidence.TrackedFiles, func(i, j int) bool { return in.Evidence.TrackedFiles[i].Path < in.Evidence.TrackedFiles[j].Path })
	return in
}
func Fingerprint(org string, in Observation) string {
	in = canonical(in)
	return digest(struct {
		Version                                                                   int
		Org, Repository, Connection, Source, SourceID, Category, Target, Advisory string
		Dependencies                                                              []detectors.Dependency
	}{1, org, in.RepositoryID, in.Evidence.ConnectionID, in.Source, in.SourceID, in.Category, in.Evidence.TargetBranch, in.Evidence.AdvisoryID, in.Evidence.Dependencies})
}
func Overlap(a, b []detectors.Dependency) bool {
	seen := map[string]bool{}
	key := func(d detectors.Dependency) string { return d.Ecosystem + "\x00" + d.Manifest + "\x00" + d.Name }
	for _, d := range a {
		seen[key(d)] = true
	}
	for _, d := range b {
		if seen[key(d)] {
			return true
		}
	}
	return false
}
