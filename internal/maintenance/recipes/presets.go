package recipes

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"reforge/internal/sandbox/guest"
)

const (
	inputMaxFiles  = 4096
	inputMaxBytes  = 32 << 20
	presetMaxFiles = 20
	presetMaxPatch = 64 << 10
	presetMaxTurns = 12
	presetTimeout  = 900
	presetMinTests = 1
)

func Build(name string, files map[string][]byte) (Recipe, error) {
	if len(files) > inputMaxFiles {
		return Recipe{}, fmt.Errorf("%w: file limit exceeded", ErrUnsupported)
	}
	total := 0
	paths := make([]string, 0, len(files))
	for file, data := range files {
		if !guest.ValidPath(file) {
			return Recipe{}, fmt.Errorf("%w: unsafe path %q", ErrUnsupported, file)
		}
		total += len(data)
		if total > inputMaxBytes {
			return Recipe{}, fmt.Errorf("%w: aggregate file limit exceeded", ErrUnsupported)
		}
		paths = append(paths, file)
	}
	sort.Strings(paths)
	var recipe Recipe
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "go", "golang":
		recipe = goPreset(paths, files)
	case "javascript", "js", "node":
		recipe = javascriptPreset(paths)
	case "python", "py":
		recipe = pythonPreset(paths, files)
	default:
		return Recipe{}, ErrUnsupported
	}
	recipe.ProtectedPaths = protected(paths)
	recipe.ManifestPaths = manifests(paths)
	if len(recipe.Commands) == 0 {
		return Recipe{}, fmt.Errorf("%w: baseline tests unavailable for %s", ErrUnsupported, strings.ToLower(strings.TrimSpace(name)))
	}
	return recipe, nil
}

func baseRecipe(name string) Recipe {
	return Recipe{Name: name, Version: "v2", MinimumTests: presetMinTests, MaxFiles: presetMaxFiles, MaxPatchBytes: presetMaxPatch, MaxTurns: presetMaxTurns, TimeoutSeconds: presetTimeout}
}

func goPreset(paths []string, files map[string][]byte) Recipe {
	r := baseRecipe("go")
	modules := make([]string, 0)
	for _, file := range paths {
		if path.Base(file) == "go.mod" && hasGoModule(files[file]) {
			modules = append(modules, path.Dir(file))
		}
	}
	for i, module := range modules {
		tests := false
		for _, file := range paths {
			if strings.HasSuffix(file, "_test.go") && within(file, module) {
				tests = true
				break
			}
		}
		if tests {
			r.Commands = append(r.Commands, Command{ID: fmt.Sprintf("go-%02d", i+1), Args: []string{"go", "test", "-json", "-count=1", "./..."}, Directory: module, TimeoutSeconds: presetTimeout, ReportFormat: "json"})
		}
	}
	return r
}

func javascriptPreset(paths []string) Recipe {
	r := baseRecipe("javascript")
	tests := make([]string, 0)
	for _, file := range paths {
		lower := strings.ToLower(file)
		if strings.HasSuffix(lower, ".test.js") || strings.HasSuffix(lower, ".spec.js") || strings.HasSuffix(lower, ".test.mjs") || strings.HasSuffix(lower, ".spec.mjs") || strings.HasSuffix(lower, ".test.cjs") || strings.HasSuffix(lower, ".spec.cjs") {
			tests = append(tests, file)
		}
	}
	if len(tests) == 0 {
		return r
	}
	for start, chunk := 0, 0; start < len(tests); chunk++ {
		end := start + 125
		if end > len(tests) {
			end = len(tests)
		}
		args := []string{"node", "--test", "--test-reporter=tap"}
		for _, test := range tests[start:end] {
			args = append(args, "./"+test)
		}
		r.Commands = append(r.Commands, Command{ID: fmt.Sprintf("node-%02d", chunk+1), Args: args, Directory: ".", TimeoutSeconds: presetTimeout, ReportFormat: "tap"})
		start = end
	}
	return r
}

func pythonPreset(paths []string, files map[string][]byte) Recipe {
	r := baseRecipe("python")
	tests := make([]string, 0)
	pytestOnly := false
	unittestEvidence := false
	for _, file := range paths {
		base := path.Base(file)
		lower := strings.ToLower(file)
		if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
			tests = append(tests, file)
		}
		content := string(files[file])
		if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") && (strings.Contains(content, "import unittest") || strings.Contains(content, "from unittest import") || strings.Contains(content, "unittest.TestCase") || strings.Contains(content, "unittest.main(")) {
			unittestEvidence = true
		}
		if base == "conftest.py" || base == "pytest.ini" || base == "tox.ini" || strings.HasSuffix(lower, "/.pytest_cache") {
			pytestOnly = true
		}
	}
	if len(tests) == 0 {
		return r
	}
	if pytestOnly || !unittestEvidence {
		return r
	}
	r.Commands = []Command{{ID: "baseline", Args: []string{"python3", "-m", "unittest", "discover", "-v"}, Directory: ".", TimeoutSeconds: presetTimeout, ReportFormat: "text"}}
	return r
}

func within(file, directory string) bool {
	return directory == "." || strings.HasPrefix(file, directory+"/")
}

func hasGoModule(data []byte) bool {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "module" {
			count++
		}
	}
	return count == 1
}

func manifests(paths []string) []string {
	result := make([]string, 0)
	for _, file := range paths {
		base := path.Base(file)
		lower := strings.ToLower(base)
		if base == "go.mod" || base == "go.sum" || base == "package.json" || base == "package-lock.json" || base == "npm-shrinkwrap.json" || base == "yarn.lock" || base == "pnpm-lock.yaml" || base == "pyproject.toml" || base == "poetry.lock" || (strings.HasPrefix(lower, "requirements") && strings.HasSuffix(lower, ".txt")) {
			result = append(result, file)
		}
	}
	return result
}

func protected(paths []string) []string {
	result := make([]string, 0)
	for _, file := range paths {
		base := path.Base(file)
		lower := strings.ToLower(file)
		name := strings.ToLower(base)
		protect := strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.js") || strings.HasSuffix(name, ".spec.js") || strings.HasSuffix(name, ".test.mjs") || strings.HasSuffix(name, ".spec.mjs") || strings.HasSuffix(name, ".test.cjs") || strings.HasSuffix(name, ".spec.cjs") || (strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"))
		protect = protect || strings.HasPrefix(lower, "tests/") || strings.Contains(lower, "/tests/") || strings.HasPrefix(lower, "testdata/") || strings.Contains(lower, "/testdata/") || strings.Contains(lower, "/__tests__/") || strings.HasPrefix(lower, "__tests__/")
		protect = protect || strings.Contains(lower, "/.github/workflows/") || strings.HasPrefix(lower, ".github/workflows/") || strings.HasPrefix(name, "package") || strings.HasSuffix(name, ".lock") || name == "go.mod" || name == "go.sum" || name == "pyproject.toml" || name == "poetry.lock" || strings.HasPrefix(name, "requirements") || name == "pytest.ini" || name == "tox.ini" || name == "setup.cfg" || name == "makefile"
		protect = protect || strings.HasPrefix(lower, "scripts/") || strings.Contains(lower, "/scripts/")
		protect = protect || name == ".gitlab-ci.yml" || strings.HasPrefix(lower, ".gitea/workflows/") || strings.Contains(lower, "/.gitea/workflows/") || name == "setup.py" || name == "conftest.py" || name == "test.py" || name == ".coveragerc"
		if protect {
			result = append(result, file)
		}
	}
	return result
}
