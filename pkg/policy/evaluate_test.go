package policy

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func number(n int64) *int64 { return &n }
func baseLayers() []Layer {
	return []Layer{{Scope: Scope{"organisation", "org"}, VersionID: "org-v1", BindingVersion: 1, Policy: Policy{Schema: "maintenance/v1", Allow: Lists{Recipes: []string{"repair"}, Models: []string{"approved"}, Routes: []string{"api"}, MergeMethods: []string{"squash"}, Environments: []string{"staging"}, Workflows: []string{"release"}}, Defaults: Defaults{Model: "approved", Route: "api"}}}}
}
func trustedInput(r Resolved, a Action) Input {
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	in := Input{Action: a, Recipe: "repair", MergeMethod: "squash", Environment: "staging", Workflow: "release", Current: Binding{Head: "head", Target: "target", Tested: "combined", PolicyHash: r.Hash, ProviderRules: "rules-v1", CapabilityVersion: "certified-v1", SourceSHA: "merged", Artifact: "sha256:artifact"}, Now: now}
	for _, id := range []string{"execution_authority", "budget_capacity", "validation", "branch_ownership", "exact_head_guard", "native_rules", "native_reviews", "native_checks", "target_enforcement", "merge_authority", "native_approvals", "artifact_provenance", "workflow_authority", "recovery_authority", "known_good_artifact"} {
		in.Evidence = append(in.Evidence, Evidence{ID: id, State: "satisfied", Binding: in.Current, ObservedAt: now, Reference: "evidence:" + id})
	}
	return in
}
func changeEvidence(in *Input, id string, fn func(*Evidence)) {
	for i := range in.Evidence {
		if in.Evidence[i].ID == id {
			fn(&in.Evidence[i])
			return
		}
	}
}

func TestPolicyCorpus(t *testing.T) {
	tests := []struct {
		name    string
		action  Action
		layers  func([]Layer) []Layer
		input   func(*Input)
		outcome string
		blocker string
	}{
		{name: "repair allows proven bounded work", action: Repair, outcome: "allow"},
		{name: "native merge with bound evidence", action: Merge, outcome: "allow"},
		{name: "deployment with provenance", action: Deploy, outcome: "allow"},
		{name: "preauthorised recovery", action: Recover, outcome: "allow"},
		{name: "ancestor path cannot be removed", action: Publish, layers: func(l []Layer) []Layer {
			l[0].Policy.ForbiddenPaths = []string{".github/**", "**/auth/*.go"}
			return append(l, Layer{Scope: Scope{"repository", "repo"}, Policy: Policy{Schema: "maintenance/v1"}})
		}, input: func(i *Input) { i.Paths = []string{".github/workflows/build.yml"} }, outcome: "deny", blocker: "forbidden path"},
		{name: "ancestor action deny wins", action: Merge, layers: func(l []Layer) []Layer { l[0].Policy.Deny = []Action{Merge}; return l }, outcome: "deny", blocker: "organisation:org"},
		{name: "explicit empty child allowlist", action: Repair, layers: func(l []Layer) []Layer {
			return append(l, Layer{Scope: Scope{"repository", "repo"}, Policy: Policy{Schema: "maintenance/v1", Allow: Lists{Recipes: []string{}}}})
		}, outcome: "deny", blocker: "recipe disallowed"},
		{name: "all team constraints intersect", action: Repair, layers: func(l []Layer) []Layer {
			return append(l, Layer{Scope: Scope{"team", "one"}, Policy: Policy{Schema: "maintenance/v1", Allow: Lists{Recipes: []string{"repair", "other"}}}}, Layer{Scope: Scope{"team", "two"}, Policy: Policy{Schema: "maintenance/v1", Allow: Lists{Recipes: []string{"other"}}}})
		}, outcome: "deny", blocker: "recipe disallowed"},
		{name: "descendant cannot raise patch ceiling", action: Publish, layers: func(l []Layer) []Layer {
			l[0].Policy.Limits.ChangedLines = number(20)
			return append(l, Layer{Scope: Scope{"repository", "repo"}, Policy: Policy{Schema: "maintenance/v1", Limits: Limits{ChangedLines: number(100)}}})
		}, input: func(i *Input) { i.Usage.ChangedLines = number(21) }, outcome: "deny", blocker: "resource ceiling"},
		{name: "missing scoped budget capacity", action: Repair, input: func(i *Input) { changeEvidence(i, "budget_capacity", func(e *Evidence) { e.State = "unknown" }) }, outcome: "unknown", blocker: "budget_capacity"},
		{name: "missing policy", action: Repair, layers: func([]Layer) []Layer { return nil }, outcome: "unknown", blocker: "organisation policy missing"},
		{name: "invalid policy", action: Repair, layers: func(l []Layer) []Layer { l[0].Policy.Schema = "future"; return l }, outcome: "unknown", blocker: "invalid policy"},
		{name: "conflicting defaults need primary team", action: Repair, layers: func(l []Layer) []Layer {
			return append(l, Layer{Scope: Scope{"team", "one"}, Policy: Policy{Schema: "maintenance/v1", Defaults: Defaults{Model: "approved"}}}, Layer{Scope: Scope{"team", "two"}, Policy: Policy{Schema: "maintenance/v1", Defaults: Defaults{Model: "other"}}})
		}, outcome: "unknown", blocker: "primary team"},
		{name: "provider API incomplete", action: Merge, input: func(i *Input) { changeEvidence(i, "native_rules", func(e *Evidence) { e.State = "incomplete" }) }, outcome: "unknown", blocker: "native_rules"},
		{name: "unsupported native enforcement", action: Merge, input: func(i *Input) { changeEvidence(i, "target_enforcement", func(e *Evidence) { e.State = "unsupported" }) }, outcome: "unknown", blocker: "target_enforcement"},
		{name: "head advanced", action: Merge, input: func(i *Input) { i.Current.Head = "new-head" }, outcome: "unknown", blocker: "stale evidence"},
		{name: "target advanced", action: Merge, input: func(i *Input) { i.Current.Target = "new-target" }, outcome: "unknown", blocker: "stale evidence"},
		{name: "combined candidate differs", action: Merge, input: func(i *Input) {
			changeEvidence(i, "validation", func(e *Evidence) { e.Binding.Tested = "old-combined" })
		}, outcome: "unknown", blocker: "validation"},
		{name: "gate weakened by candidate", action: Merge, input: func(i *Input) { changeEvidence(i, "validation", func(e *Evidence) { e.State = "failed" }) }, outcome: "deny", blocker: "validation"},
		{name: "stale approvals", action: Merge, input: func(i *Input) {
			changeEvidence(i, "native_reviews", func(e *Evidence) { e.ObservedAt = i.Now.Add(-time.Hour) })
		}, outcome: "unknown", blocker: "native_reviews"},
		{name: "unknown enum fails closed", action: Merge, input: func(i *Input) { changeEvidence(i, "native_checks", func(e *Evidence) { e.State = "probably_good" }) }, outcome: "unknown", blocker: "native_checks"},
		{name: "bot merge cooperation unproven", action: Merge, input: func(i *Input) { changeEvidence(i, "merge_authority", func(e *Evidence) { e.State = "unknown" }) }, outcome: "unknown", blocker: "merge_authority"},
		{name: "bot branch moved", action: Publish, input: func(i *Input) { changeEvidence(i, "exact_head_guard", func(e *Evidence) { e.State = "failed" }) }, outcome: "deny", blocker: "exact_head_guard"},
		{name: "dependency pause", action: Repair, input: func(i *Input) { i.PausedScopes = []string{"runner_pool"} }, outcome: "deny", blocker: "paused"},
		{name: "parent pause", action: Publish, layers: func(l []Layer) []Layer { l[0].Policy.Paused = true; return l }, outcome: "deny", blocker: "paused"},
		{name: "connection revoked or stale lease", action: Publish, input: func(i *Input) { changeEvidence(i, "execution_authority", func(e *Evidence) { e.State = "failed" }) }, outcome: "deny", blocker: "execution_authority"},
		{name: "artifact differs", action: Deploy, input: func(i *Input) {
			changeEvidence(i, "artifact_provenance", func(e *Evidence) { e.Binding.Artifact = "wrong" })
		}, outcome: "unknown", blocker: "artifact_provenance"},
		{name: "native approval absent", action: Deploy, input: func(i *Input) { changeEvidence(i, "native_approvals", func(e *Evidence) { e.State = "failed" }) }, outcome: "deny", blocker: "native_approvals"},
		{name: "recovery unapproved", action: Recover, input: func(i *Input) { changeEvidence(i, "recovery_authority", func(e *Evidence) { e.State = "unknown" }) }, outcome: "unknown", blocker: "recovery_authority"},
		{name: "prior policy cannot grandfather authority", action: Merge, input: func(i *Input) { i.StartingPolicyHash = "prior"; i.Current.PolicyHash = "prior" }, outcome: "unknown", blocker: "policy binding"},
		{name: "traversal rejected", action: Publish, input: func(i *Input) { i.Paths = []string{"src/../../secret"} }, outcome: "deny", blocker: "invalid candidate path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layers := baseLayers()
			if tt.layers != nil {
				layers = tt.layers(layers)
			}
			r := Resolve(layers, "repo", "", false)
			in := trustedInput(r, tt.action)
			if tt.input != nil {
				tt.input(&in)
			}
			got := Evaluate(r, in)
			if got.Outcome != tt.outcome || !strings.Contains(strings.Join(got.Blockers, ";"), tt.blocker) {
				t.Fatalf("decision=%+v; want %s containing %q", got, tt.outcome, tt.blocker)
			}
		})
	}
}

func TestApprovalRulesAndPublisherIdentity(t *testing.T) {
	layers := baseLayers()
	layers[0].Policy.Required = []Requirement{{ID: "security", Identity: "app:security-scanner", Actions: []Action{Merge}, Approvals: 2}, {ID: "operations", Actions: []Action{Merge}, Approvals: 1}}
	layers = append(layers, Layer{Scope: Scope{"repository", "repo"}, Policy: Policy{Schema: "maintenance/v1", Required: []Requirement{{ID: "security", Identity: "app:security-scanner", Actions: []Action{Merge}, Approvals: 1}}}})
	r := Resolve(layers, "repo", "", false)
	in := trustedInput(r, Merge)
	for _, id := range []string{"security", "operations"} {
		identity := ""
		if id == "security" {
			identity = "app:security-scanner"
		}
		in.Evidence = append(in.Evidence, Evidence{ID: id, Identity: identity, State: "satisfied", Approvals: 1, ObservedAt: in.Now, Binding: in.Current, Reference: id})
	}
	if got := Evaluate(r, in); got.Outcome != "deny" {
		t.Fatalf("ancestor approval count lost: %+v", got)
	}
	changeEvidence(&in, "security", func(e *Evidence) { e.Approvals = 2 })
	if got := Evaluate(r, in); got.Outcome != "allow" {
		t.Fatalf("valid distinct rules denied: %+v", got)
	}
	changeEvidence(&in, "security", func(e *Evidence) { e.Identity = "attacker-app" })
	if got := Evaluate(r, in); got.Outcome == "allow" {
		t.Fatal("same-named check from wrong publisher accepted")
	}
}

func TestCanonicalPolicyAndPrimaryDefaults(t *testing.T) {
	layers := baseLayers()
	layers[0].Policy.Allow.Recipes = []string{"other", "repair"}
	layers = append(layers, Layer{Scope: Scope{"team", "team"}, Policy: Policy{Schema: "maintenance/v1", Defaults: Defaults{Model: "approved"}}})
	r := Resolve(layers, "repo", "team", false)
	copy := []Layer{layers[1], layers[0]}
	copy[1].Policy.Allow.Recipes = []string{"repair", "other"}
	if other := Resolve(copy, "repo", "team", false); other.Hash != r.Hash {
		t.Fatal("ordering changed canonical hash")
	}
	before, _ := json.Marshal(layers)
	if got := Evaluate(r, trustedInput(r, Repair)); got.Outcome != "allow" {
		t.Fatalf("primary default unresolved: %+v", got)
	}
	if after, _ := json.Marshal(layers); !reflect.DeepEqual(before, after) {
		t.Fatal("evaluation mutated inputs")
	}
	if got := Evaluate(Resolve(nil, "repo", "", true), Input{Action: Read}); got.Outcome != "allow" {
		t.Fatal("inventory blocked by missing/paused policy")
	}
}

func TestPolicyInputValidation(t *testing.T) {
	for _, data := range []string{`{"schema":"maintenance/v1","script":"allow()"}`, `{"schema":"maintenance/v1","limits":{"attempts":-1}}`, `{"schema":"maintenance/v1","deny":["arbitrary"]}`, `{"schema":"maintenance/v1"} {}`} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Fatalf("invalid policy accepted: %s", data)
		}
	}
	for _, tt := range []struct {
		pattern, file string
		want          bool
	}{{"**/auth/*.go", "internal/auth/service.go", true}, {".github/**", ".github/workflows/test.yml", true}, {"*.go", "internal/auth/service.go", false}, {"**/test.go", "test.go", true}} {
		if forbidden(tt.pattern, tt.file) != tt.want {
			t.Fatalf("path match %s %s", tt.pattern, tt.file)
		}
	}
}

func TestEffectivePolicyAndSnapshotIsolation(t *testing.T) {
	layers := baseLayers()
	layers[0].Policy.Deny = []Action{Deploy}
	layers[0].Policy.ForbiddenPaths = []string{".github/**"}
	layers[0].Policy.Limits.Attempts = number(3)
	layers[0].Policy.Required = []Requirement{{ID: "security", Identity: "scanner", Actions: []Action{Merge}, Approvals: 2}}
	layers = append(layers, Layer{Scope: Scope{"repository", "repo"}, Policy: Policy{Schema: "maintenance/v1", Deny: []Action{Deploy, Recover}, ForbiddenPaths: []string{".github/**", "infra/**"}, Required: []Requirement{{ID: "security", Identity: "scanner", Actions: []Action{Merge, Publish}, Approvals: 1}}}})
	r := Resolve(layers, "repo", "", false)
	if len(r.Policy.Deny) != 2 || len(r.Policy.ForbiddenPaths) != 2 || len(r.Policy.Required) != 2 {
		t.Fatalf("effective constraints omitted: %+v", r.Policy)
	}
	for _, req := range r.Policy.Required {
		if applies(req.Actions, Merge) && req.Approvals != 2 {
			t.Fatal("effective approval maximum lost")
		}
		if applies(req.Actions, Publish) && req.Approvals != 1 {
			t.Fatal("approval count leaked across actions")
		}
	}
	*layers[0].Policy.Limits.Attempts = 100
	layers[0].Policy.ForbiddenPaths[0] = "changed"
	if *r.Policy.Limits.Attempts != 3 || r.Layers[0].Policy.ForbiddenPaths[0] != ".github/**" {
		t.Fatal("resolved snapshot aliases caller")
	}
}

func TestGlobRepeatedWildcardsAreBounded(t *testing.T) {
	pattern := strings.Repeat("**/", 30) + "absent"
	file := strings.Repeat("nested/", 100) + "present"
	if forbidden(pattern, file) {
		t.Fatal("wildcard unexpectedly matched")
	}
	layers := baseLayers()
	layers[0].Policy.ForbiddenPaths = []string{strings.Repeat("**/", 33) + "file"}
	if err := Validate(layers[0].Policy); err == nil {
		t.Fatal("unbounded pattern accepted")
	}
	r := Resolve(baseLayers(), "repo", "", false)
	in := trustedInput(r, Publish)
	in.Paths = make([]string, 201)
	if got := Evaluate(r, in); got.Outcome != "deny" {
		t.Fatal("oversized candidate input accepted")
	}
}

func TestDistinctPublisherRequirements(t *testing.T) {
	layers := baseLayers()
	layers[0].Policy.Required = []Requirement{{ID: "security", Identity: "first", Actions: []Action{Merge}, Approvals: 1}, {ID: "security", Identity: "second", Actions: []Action{Merge}, Approvals: 2}}
	r := Resolve(layers, "repo", "", false)
	in := trustedInput(r, Merge)
	for _, identity := range []string{"first", "second"} {
		in.Evidence = append(in.Evidence, Evidence{ID: "security", Identity: identity, State: "satisfied", Approvals: 2, Binding: in.Current, ObservedAt: in.Now, Reference: identity})
	}
	if got := Evaluate(r, in); got.Outcome != "allow" || !contains(got.Rules, "security@second") {
		t.Fatalf("distinct publisher rules conflated: %+v", got)
	}
	in.Evidence = in.Evidence[:len(in.Evidence)-1]
	if got := Evaluate(r, in); got.Outcome != "unknown" {
		t.Fatal("one publisher satisfied another's requirement")
	}
}
