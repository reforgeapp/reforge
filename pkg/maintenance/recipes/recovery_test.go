package recipes

import (
	"slices"
	"testing"
)

func TestCIGapOffersLanguageRepairsBeforeBootstrap(t *testing.T) {
	got := ForFinding("ci_gap", "go")
	bootstrap := slices.Index(got, "bootstrap")
	if bootstrap < 0 {
		t.Fatalf("ci_gap recipes omit bootstrap: %v", got)
	}
	for _, name := range []string{"go", "javascript", "python"} {
		index := slices.Index(got, name)
		if index < 0 || index > bootstrap {
			t.Fatalf("ci_gap recipe %q missing or ordered after bootstrap: %v", name, got)
		}
	}
	if got := ForFinding("missing_validation", "go"); !slices.Equal(got, []string{"bootstrap"}) {
		t.Fatalf("missing-validation route changed: %v", got)
	}
}
