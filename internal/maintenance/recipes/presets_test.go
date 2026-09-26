package recipes

import "testing"

func TestBuildPresetsCorpus(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string][]byte
		command []string
	}{
		{"go", map[string][]byte{"go.mod": []byte("module example.test\n"), "z_test.go": nil, ".github/workflows/test.yml": nil}, []string{"go", "test", "-json", "-count=1", "./..."}},
		{"javascript", map[string][]byte{"package.json": nil, "tests/z.spec.js": nil, "tests/a.test.js": nil, "package-lock.json": nil}, []string{"node", "--test", "--test-reporter=tap", "./tests/a.test.js", "./tests/z.spec.js"}},
		{"python", map[string][]byte{"pyproject.toml": nil, "tests/test_z.py": []byte("import unittest\n"), "tests/test_a.py": []byte("from unittest import TestCase\n")}, []string{"python3", "-m", "unittest", "discover", "-v"}},
	}
	for _, tc := range tests {
		r, err := Build(tc.name, tc.files)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(r.Commands) != 1 {
			t.Fatalf("%s command count: %#v", tc.name, r.Commands)
		}
		for i, arg := range tc.command {
			if r.Commands[0].Args[i] != arg {
				t.Fatalf("%s args: %#v", tc.name, r.Commands[0].Args)
			}
		}
		if r.MaxFiles != 20 || r.MaxPatchBytes != 64<<10 || r.MaxTurns != 16 || r.Version != "v3" || r.TimeoutSeconds != 900 || r.MinimumTests != 1 {
			t.Fatalf("%s bounds: %#v", tc.name, r)
		}
	}
}

func TestBuildRejectsUnsafeAndUnsupportedBaselines(t *testing.T) {
	for _, files := range []map[string][]byte{{"../x_test.go": nil}, {"x.go": nil}, {"tests/test_app.py": nil, "pytest.ini": nil}, {"package.json": nil}} {
		if _, err := Build("go", files); err == nil {
			t.Fatalf("unsafe or unsupported input accepted: %#v", files)
		}
	}
	if _, err := Build("python", map[string][]byte{"tests/test_app.py": []byte("def test_app(): pass\n")}); err == nil {
		t.Fatal("pytest-style-only layout accepted")
	}
}

func TestBuildInventoryBoundAndNodePathSafety(t *testing.T) {
	files := map[string][]byte{"package.json": nil, "-bad.test.js": nil}
	for i := 0; i < 25; i++ {
		files["src/file"+string(rune('a'+i))+".js"] = nil
	}
	r, err := Build("javascript", files)
	if err != nil {
		t.Fatal(err)
	}
	if r.MaxFiles != 20 || len(r.Commands) != 1 || r.Commands[0].Args[3] != "./-bad.test.js" {
		t.Fatalf("inventory/path handling: %#v", r)
	}
}

func TestBuildNodeChunksSandboxArguments(t *testing.T) {
	files := map[string][]byte{"package.json": nil}
	for i := 0; i < 130; i++ {
		files["tests/test"+string(rune('a'+i/26))+string(rune('a'+i%26))+".test.js"] = nil
	}
	r, err := Build("javascript", files)
	if err != nil || len(r.Commands) != 2 {
		t.Fatalf("chunks: %v %#v", err, r.Commands)
	}
	for i, command := range r.Commands {
		if len(command.Args) > 128 || command.ID != ("node-0"+string(rune('1'+i))) {
			t.Fatalf("chunk %d: %#v", i, command)
		}
	}
}

func TestBuildBoundsAndProtectedPaths(t *testing.T) {
	files := map[string][]byte{"go.mod": []byte("module example.test\n"), "go.sum": nil, "app_test.go": nil, "Makefile": nil, ".github/workflows/check.yml": nil}
	r, err := Build("go", files)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.ManifestPaths) != 2 || len(r.ProtectedPaths) != 5 {
		t.Fatalf("paths: %#v %#v", r.ManifestPaths, r.ProtectedPaths)
	}
	for i := 1; i < len(r.ProtectedPaths); i++ {
		if r.ProtectedPaths[i-1] > r.ProtectedPaths[i] {
			t.Fatal("protected paths unsorted")
		}
	}
}

func TestBuildOwnerAggregatesMixedRepositoryCommands(t *testing.T) {
	files := map[string][]byte{
		"go.mod":            []byte("module example.test\n"),
		"app_test.go":       nil,
		"tests/test_app.py": []byte("import unittest\nclass TestApp(unittest.TestCase): pass\n"),
		"package.json":      []byte(`{"scripts":{"lint":"eslint .","typecheck":"vue-tsc --noEmit","build":"vite build","test:unit":"vitest"}}`),
		"src/App.test.ts":   nil,
	}
	r, err := BuildOwner("go", files)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "go" || r.Version != "v4" || len(r.Commands) != 6 {
		t.Fatalf("recipe: %#v", r)
	}
	if r.Commands[0].ID != "go-01" || r.Commands[0].ReportFormat != "json" || r.Commands[1].ID != "baseline" || r.Commands[2].ID != "js-01" || r.Commands[5].Args[len(r.Commands[5].Args)-1] != "--run" {
		t.Fatalf("commands: %#v", r.Commands)
	}
	if r.Commands[2].Args[0] != "node" || r.Commands[2].Args[1] != "/usr/local/lib/node_modules/npm/bin/npm-cli.js" || r.Commands[2].ReportFormat != "exit" {
		t.Fatalf("package command: %#v", r.Commands[2])
	}
}

func TestBuildOwnerJavaScriptOnlyVitestAndUnsupportedEmpty(t *testing.T) {
	r, err := BuildOwner("go", map[string][]byte{
		"package.json":     []byte(`{"scripts":{"test":"node ./node_modules/vitest/vitest.mjs run src/**/*.test.ts"}}`),
		"src/view.test.ts": nil,
	})
	if err != nil || len(r.Commands) != 1 || r.Commands[0].Args[len(r.Commands[0].Args)-1] != "test" {
		t.Fatalf("vitest recipe: %v %#v", err, r.Commands)
	}
	if _, err := BuildOwner("go", map[string][]byte{"package.json": []byte(`{"scripts":{}}`)}); err == nil {
		t.Fatal("empty project accepted")
	}
	if _, err := BuildOwner("owner", map[string][]byte{"package.json": []byte(`{"scripts":{"test":"node ./node_modules/vitest/vitest.mjs run src/**/*.test.ts"}}`)}); err == nil {
		t.Fatal("unsupported recipe name accepted")
	}
}

func TestBuildOwnerBareJavaScriptTests(t *testing.T) {
	t.Run("javascript only", func(t *testing.T) {
		r, err := BuildOwner("javascript", map[string][]byte{"tests/app.test.js": nil})
		if err != nil || len(r.Commands) != 1 || r.Commands[0].ID != "node-01" || r.Commands[0].ReportFormat != "tap" {
			t.Fatalf("bare JavaScript recipe: %v %#v", err, r.Commands)
		}
	})
	t.Run("mixed Go and JavaScript", func(t *testing.T) {
		r, err := BuildOwner("go", map[string][]byte{
			"go.mod":            []byte("module example.test\n"),
			"app_test.go":       nil,
			"tests/app.test.js": nil,
		})
		if err != nil || len(r.Commands) != 2 || r.Commands[0].ID != "go-01" || r.Commands[1].ID != "node-01" {
			t.Fatalf("mixed recipe: %v %#v", err, r.Commands)
		}
	})
}

func TestBuildOwnerPackageRunnerOwnsJavaScriptTestTree(t *testing.T) {
	r, err := BuildOwner("javascript", map[string][]byte{
		"package.json":    []byte(`{"scripts":{"test":"jest"}}`),
		"src/app.test.js": nil,
	})
	if err != nil || len(r.Commands) != 1 || r.Commands[0].Args[3] != "test" {
		t.Fatalf("package test recipe: %v %#v", err, r.Commands)
	}
}
