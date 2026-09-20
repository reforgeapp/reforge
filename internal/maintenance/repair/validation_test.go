package repair

import (
	"strings"
	"testing"

	"reforge/internal/maintenance/recipes"
	"reforge/internal/sandbox"
)

func testPlan(t *testing.T) (Plan, map[string][]byte) {
	t.Helper()
	files := map[string][]byte{"value.js": []byte("exports.add = (a,b) => a-b"), "value.test.js": []byte("const test = require('node:test'); const assert = require('node:assert'); test('adds',()=>assert.equal(require('./value').add(2,3),5))"), "package.json": []byte(`{"scripts":{"test":"node --test"}}`)}
	p, err := Freeze("javascript", "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 40), strings.Repeat("c", 40), files, []string{"private/**"})
	if err != nil {
		t.Fatal(err)
	}
	return p, files
}
func TestPatchCannotChangeValidationOrSensitivePaths(t *testing.T) {
	p, files := testPlan(t)
	for _, patch := range []sandbox.Patch{
		{Path: "value.test.js", Content: []byte("pass")}, {Path: "package.json", Content: []byte("{}")}, {Path: "value.js", Delete: true}, {Path: "scripts/check.js", Content: []byte("pass")}, {Path: "private/source.js", Content: []byte("pass")}, {Path: "value.js", Content: []byte("process.exit(0)")}, {Path: ".git/config", Content: []byte("x")}, {Path: "value.js", Content: []byte(strings.Repeat("x", p.Recipe.MaxPatchBytes+1))},
	} {
		if CheckPatch(p, files, []sandbox.Patch{patch}) == nil {
			t.Fatalf("unsafe patch accepted: %s", patch.Path)
		}
	}
	if err := CheckPatch(p, files, []sandbox.Patch{{Path: "latest.js", Content: []byte("exports.add = (a,b) => a+b")}}); err != nil {
		t.Fatal(err)
	}
	files["value.test.js"] = []byte("changed")
	if Protected(p, files) {
		t.Fatal("changed test accepted")
	}
	p.Recipe.Commands[0].Args = []string{"true"}
	if p.Valid() {
		t.Fatal("altered plan accepted")
	}
}
func TestValidationRejectsNoopSkippedLostAndUnreproducedChecks(t *testing.T) {
	p, _ := testPlan(t)
	command := p.Recipe.Commands[0]
	failed := Interpret(command, sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - adds\n")})
	passed := Interpret(command, sandbox.CommandResult{ExitCode: 0, Output: []byte("ok 1 - adds\n")})
	if !Reproduced([]CheckResult{failed}) || !Verified(p, []CheckResult{failed}, []CheckResult{passed}) {
		t.Fatal("real failure/pass rejected")
	}
	for _, raw := range []sandbox.CommandResult{
		{Output: []byte("done")}, {Output: []byte("ok 1 - adds # SKIP disabled\n")}, {Output: []byte("ok 1 - different\n")}, {Output: []byte("ok 1 - adds\n"), Truncated: true}, {Output: []byte("ok 1 - adds\n"), TimedOut: true}, {ExitCode: 127, Output: []byte("not found")},
	} {
		out := Interpret(command, raw)
		if Verified(p, []CheckResult{failed}, []CheckResult{out}) {
			t.Fatalf("invalid validation accepted: %+v", raw)
		}
	}
	if Reproduced([]CheckResult{passed}) || Reproduced([]CheckResult{Interpret(command, sandbox.CommandResult{ExitCode: 127})}) {
		t.Fatal("unreproduced environment accepted")
	}
}
func TestGoAndPythonReportsRetainCaseIdentity(t *testing.T) {
	checks := []struct{ format, body, key string }{
		{"json", "{\"Action\":\"pass\",\"Package\":\"fixture\",\"Test\":\"TestAdd\"}\n", "fixture/TestAdd"},
		{"text", "test_add (test_value.ValueTest.test_add) ... ok\n", "test_add (test_value.ValueTest.test_add)"},
	}
	for _, check := range checks {
		r := Interpret(recipes.Command{ID: "check", ReportFormat: check.format}, sandbox.CommandResult{Output: []byte(check.body)})
		if !r.Complete || r.Cases[check.key] != "pass" {
			t.Fatalf("case lost: %+v", r)
		}
	}
}
