package forge

import (
	"reflect"
	"strings"
	"testing"
)

func TestMissingChecks(t *testing.T) {
	bot := func(name string) bool { return strings.EqualFold(name, "dependabot") }
	target := []Check{{Name: "Validate", Conclusion: "success"}, {Name: "Gitleaks", Conclusion: "success"}, {Name: "Dependabot", Conclusion: "success"}, {Name: "Flaky", Conclusion: "failure"}}
	missing, pending := MissingChecks(target, []Check{{Name: "Validate", Conclusion: "success"}}, bot)
	if pending || !reflect.DeepEqual(missing, []string{"Gitleaks"}) {
		t.Fatalf("dropped check: %v %v", missing, pending)
	}
	missing, _ = MissingChecks(target, []Check{{Name: "Validate", Conclusion: "success"}, {Name: "Gitleaks", Conclusion: "success"}, {Name: "Gitleaks", Conclusion: "failure"}}, bot)
	if !reflect.DeepEqual(missing, []string{"Gitleaks"}) {
		t.Fatalf("failing rerun counted as passing: %v", missing)
	}
	missing, pending = MissingChecks(target, []Check{{Name: "Validate", Conclusion: "success"}, {Name: "Gitleaks"}}, bot)
	if !pending || len(missing) != 1 {
		t.Fatalf("pending check: %v %v", missing, pending)
	}
}
