package repair

import (
	"testing"

	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
)

func TestLostPassingCase(t *testing.T) {
	base := []CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "pass", "3:flaky": "fail"}}}
	for name, tc := range map[string]struct {
		candidate []CheckResult
		lost      bool
	}{
		"kept":        {[]CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "pass", "3:flaky": "pass"}}}, false},
		"renumbered":  {[]CheckResult{{Cases: map[string]string{"1:new": "pass", "2:adds": "pass", "3:subtracts": "pass"}}}, false},
		"deleted":     {[]CheckResult{{Cases: map[string]string{"1:adds": "pass"}}}, true},
		"now-failing": {[]CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "fail"}}}, true},
	} {
		if got := lostPassingCase(base, tc.candidate); got != tc.lost {
			t.Errorf("%s: lost=%t", name, got)
		}
	}
}

func TestGoCasesRemainComparableAcrossModulePathChange(t *testing.T) {
	command := recipes.Command{Directory: ".", Args: []string{"go", "test", "-json", "./..."}, ReportFormat: "json"}
	baseline := []CheckResult{{Cases: map[string]string{
		"old.example/repo/TestAdd":               "pass",
		"old.example/repo/internal/math/TestAdd": "pass",
		"old.example/repo/TestRoutes/a/../b":     "pass",
		"old.example/repo/TestRoutes/b":          "pass",
	}}}
	candidate := []CheckResult{{Cases: map[string]string{
		"new.example/repo/TestAdd":               "pass",
		"new.example/repo/internal/math/TestAdd": "pass",
		"new.example/repo/TestRoutes/a/../b":     "pass",
		"new.example/repo/TestRoutes/b":          "pass",
	}}}
	baseline = canonicalizeGoCases(baseline, []recipes.Command{command}, map[string][]byte{"go.mod": []byte("module old.example/repo\n")})
	candidate = canonicalizeGoCases(candidate, []recipes.Command{command}, map[string][]byte{"go.mod": []byte("module new.example/repo\n")})
	if lostPassingCase(baseline, candidate) {
		t.Fatal("module path change made passing tests appear lost")
	}
	if len(baseline[0].Cases) != 4 || baseline[0].Cases["TestRoutes/a/../b"] != "pass" || baseline[0].Cases["TestRoutes/b"] != "pass" {
		t.Fatalf("test name suffix was altered or collapsed: %+v", baseline[0].Cases)
	}
	candidate[0].Cases["TestAdd"] = "skip"
	if !lostPassingCase(baseline, candidate) {
		t.Fatal("module path normalization accepted a skipped baseline pass")
	}
}
