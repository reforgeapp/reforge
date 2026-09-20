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
		if r.MaxFiles != 20 || r.MaxPatchBytes != 64<<10 || r.MaxTurns != 12 || r.Version != "v2" || r.TimeoutSeconds != 900 || r.MinimumTests != 1 {
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
