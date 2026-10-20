package detectors

import "testing"

func TestPythonAmbiguousFieldsAndExtrasIdentity(t *testing.T) {
	for _, content := range []string{"[project]\ndependencies=[1, 'requests>=2']", "[project]\ndynamic=['optional-dependencies']"} {
		if Compare(nil, map[string][]byte{"pyproject.toml": []byte(content)}).Complete {
			t.Fatal("ambiguous metadata treated as complete")
		}
	}
	r := Compare(map[string][]byte{"pyproject.toml": []byte("[project]\ndependencies=['Requests[security]>=2']")}, map[string][]byte{"pyproject.toml": []byte("[project]\ndependencies=['requests>=3']")})
	if !r.Complete || len(r.Changes) != 1 || r.Changes[0].Name != "requests" || r.Changes[0].From != "[security]>=2" {
		t.Fatal("extras split package identity", r)
	}
	if BotConfiguration(map[string][]byte{"renovate.json": []byte(`{"automerge":true,"automerge":false}`)}).Renovate.AutoMerge != "unknown" {
		t.Fatal("duplicate key manufactured disabled automerge")
	}
}

func TestCompareMeaningfulCorpus(t *testing.T) {
	base := map[string][]byte{
		"go.mod":               []byte("module example\nrequire (\n\tgolang.org/x/net v0.1.0\n\tgolang.org/x/text v0.2.0\n)\n"),
		"package.json":         []byte(`{"dependencies":{"react":"^18.0.0","old":"1.0.0"},"devDependencies":{"test":"~2.0.0"}}`),
		"pyproject.toml":       []byte("[project]\ndependencies = [\"Requests>=2.0\"]\n"),
		"requirements-dev.txt": []byte("pytest==7.0\n"),
	}
	candidate := map[string][]byte{
		"go.mod":               []byte("module example\nrequire (\n\tgolang.org/x/net v0.2.0\n)\n"),
		"package.json":         []byte(`{"dependencies":{"react":"^18.0.0","new":"2.0.0"},"devDependencies":{"test":"~2.1.0"}}`),
		"pyproject.toml":       []byte("[project]\ndependencies = [\"requests>=2.1\"]\n"),
		"requirements-dev.txt": []byte("pytest==8.0\n"),
	}
	r := Compare(base, candidate)
	if !r.Complete || len(r.Reasons) != 0 {
		t.Fatalf("unexpected incomplete result: %#v", r)
	}
	if len(r.Changes) != 7 {
		t.Fatalf("got %d changes, want 7: %#v", len(r.Changes), r.Changes)
	}
	for i := 1; i < len(r.Changes); i++ {
		if identity(r.Changes[i-1]) > identity(r.Changes[i]) {
			t.Fatalf("changes not deterministic: %#v", r.Changes)
		}
	}
}

func TestCompareRejectsAmbiguityAndDuplicateKeys(t *testing.T) {
	r := Compare(nil, map[string][]byte{"package.json": []byte(`{"dependencies":{"x":"1"},"dependencies":{"x":"2"}}`)})
	if r.Complete || len(r.Reasons) == 0 {
		t.Fatalf("ambiguous input accepted: %#v", r)
	}
}

func TestRequirementsOptionsAreIncomplete(t *testing.T) {
	for _, option := range []string{"-e git+https://example.invalid/x", "-r base.txt", "--index-url https://example.invalid/simple"} {
		r := Compare(nil, map[string][]byte{"requirements.txt": []byte(option + "\n")})
		if r.Complete || len(r.Reasons) == 0 {
			t.Fatalf("option accepted: %q", option)
		}
	}
	for _, requirement := range []string{"https://example.invalid/pkg.whl", "pkg @ git+https://example.invalid/pkg", "requests>=2 trailing"} {
		r := Compare(nil, map[string][]byte{"requirements.txt": []byte(requirement + "\n")})
		if r.Complete || len(r.Reasons) == 0 {
			t.Fatalf("ambiguous requirement accepted: %q", requirement)
		}
	}
}

func TestGoUnsupportedAndUnterminatedSyntaxIsIncomplete(t *testing.T) {
	for _, content := range []string{"module example\nreplace example => other\n", "module example\nrequire (\n example v1.0.0\n"} {
		r := Compare(nil, map[string][]byte{"go.mod": []byte(content)})
		if r.Complete || len(r.Reasons) == 0 {
			t.Fatalf("go syntax accepted: %q", content)
		}
	}
}

func TestBotConfigurationDynamicExtendsUnknown(t *testing.T) {
	r := BotConfiguration(map[string][]byte{
		"renovate.json":          []byte(`{"extends":["config:recommended"]}`),
		".github/dependabot.yml": []byte("version: 2\nupdates:\n- package-ecosystem: gomod\n  directory: /\n  automerge: true\n"),
	})
	if !r.Renovate.Present || r.Renovate.AutoMerge != "unknown" {
		t.Fatalf("renovate: %#v", r.Renovate)
	}
	if !r.Dependabot.Present || r.Dependabot.AutoMerge != "unknown" {
		t.Fatalf("dependabot: %#v", r.Dependabot)
	}
	presence := BotConfiguration(map[string][]byte{".renovaterc.json5": []byte("// json5"), "renovate.config.js": []byte("module.exports = {}")})
	if !presence.Renovate.Present || presence.Renovate.AutoMerge != "unknown" {
		t.Fatalf("renovate alternate configs: %#v", presence.Renovate)
	}
}

func TestPythonRangesMarkersAndComments(t *testing.T) {
	result := Compare(nil, map[string][]byte{"requirements.txt": []byte("requests[socks]>=2.31,<3; python_version >= '3.11' # reason\n")})
	if !result.Complete || len(result.Changes) != 1 || result.Changes[0].Name != "requests" {
		t.Fatalf("valid requirement rejected: %+v", result)
	}
}
