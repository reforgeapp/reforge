package recipes

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"reforge/internal/sandbox/guest"
)

const (
	CurrentVersion = "v3"
	inputMaxFiles  = 4096
	inputMaxBytes  = 64 << 20
	presetMaxFiles = 20
	presetMaxPatch = 64 << 10
	presetMaxTurns = 16
	presetTimeout  = 900
	presetMinTests = 1
)

func Build(name string, files map[string][]byte) (Recipe, error) {
	paths, err := inventory(files)
	if err != nil {
		return Recipe{}, err
	}

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

func BuildOwner(name string, files map[string][]byte) (Recipe, error) {
	paths, err := inventory(files)
	if err != nil {
		return Recipe{}, err
	}
	canonical, ok := canonicalName(name)
	if !ok {
		return Recipe{}, ErrUnsupported
	}
	if a, _ := Lookup(canonical); a.Validator != "" {
		return structuralRecipe(a), nil
	}
	r := baseRecipe(canonical)
	r.Version = "v4"
	r.Commands = append(r.Commands, goPreset(paths, files).Commands...)
	r.Commands = append(r.Commands, pythonPreset(paths, files).Commands...)
	jsCommands, coveredDirs := ownerJavaScriptCommands(paths, files)
	r.Commands = append(r.Commands, jsCommands...)
	r.Commands = append(r.Commands, ownerNodeTestCommands(paths, coveredDirs)...)
	r.ProtectedPaths = protected(paths)
	r.ManifestPaths = manifests(paths)
	if len(r.Commands) == 0 {
		return Recipe{}, fmt.Errorf("%w: baseline tests unavailable for %s", ErrUnsupported, strings.ToLower(strings.TrimSpace(name)))
	}
	return r, nil
}

func canonicalName(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "go", "golang":
		return "go", true
	case "javascript", "js", "node":
		return "javascript", true
	case "python", "py":
		return "python", true
	case "config", "bootstrap":
		return strings.ToLower(strings.TrimSpace(name)), true
	default:
		return "", false
	}
}

func inventory(files map[string][]byte) ([]string, error) {
	if len(files) > inputMaxFiles {
		return nil, fmt.Errorf("%w: file limit exceeded", ErrUnsupported)
	}
	total := 0
	paths := make([]string, 0, len(files))
	for file, data := range files {
		if !guest.ValidPath(file) {
			return nil, fmt.Errorf("%w: unsafe path %q", ErrUnsupported, file)
		}
		total += len(data)
		if total > inputMaxBytes {
			return nil, fmt.Errorf("%w: aggregate file limit exceeded", ErrUnsupported)
		}
		paths = append(paths, file)
	}
	sort.Strings(paths)
	return paths, nil
}

func ownerJavaScriptCommands(paths []string, files map[string][]byte) ([]Command, []string) {
	commands := make([]Command, 0)
	coveredDirs := make([]string, 0)
	for _, file := range paths {
		if path.Base(file) != "package.json" {
			continue
		}
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(files[file], &pkg) != nil || len(pkg.Scripts) == 0 {
			continue
		}
		directory := path.Dir(file)
		if directory == "" {
			directory = "."
		}
		for _, script := range []string{"lint", "typecheck", "build"} {
			if strings.TrimSpace(pkg.Scripts[script]) != "" {
				commands = append(commands, npmCommand(script, directory, nil, len(commands)+1))
			}
		}
		if strings.TrimSpace(pkg.Scripts["test:unit"]) != "" {
			commands = append(commands, npmCommand("test:unit", directory, testRunnerArgs(pkg.Scripts["test:unit"]), len(commands)+1))
			coveredDirs = append(coveredDirs, directory)
			continue
		}
		testScript := strings.TrimSpace(pkg.Scripts["test"])
		if testScript == "" {
			continue
		}
		coveredDirs = append(coveredDirs, directory)
		args, supported := testRunnerArgs(testScript), supportedTestRunner(testScript)
		if supported {
			commands = append(commands, npmCommand("test", directory, args, len(commands)+1))
		}
	}
	return commands, coveredDirs
}

func ownerNodeTestCommands(paths, coveredDirs []string) []Command {
	tests := make([]string, 0)
	for _, file := range paths {
		lower := strings.ToLower(file)
		if !strings.HasSuffix(lower, ".test.js") && !strings.HasSuffix(lower, ".spec.js") && !strings.HasSuffix(lower, ".test.mjs") && !strings.HasSuffix(lower, ".spec.mjs") && !strings.HasSuffix(lower, ".test.cjs") && !strings.HasSuffix(lower, ".spec.cjs") {
			continue
		}
		covered := false
		for _, directory := range coveredDirs {
			if within(file, directory) {
				covered = true
				break
			}
		}
		if !covered {
			tests = append(tests, file)
		}
	}
	commands := make([]Command, 0, (len(tests)+124)/125)
	for start, chunk := 0, 0; start < len(tests); chunk++ {
		end := start + 125
		if end > len(tests) {
			end = len(tests)
		}
		args := []string{"node", "--test", "--test-reporter=tap"}
		for _, test := range tests[start:end] {
			args = append(args, "./"+test)
		}
		commands = append(commands, Command{ID: fmt.Sprintf("node-%02d", chunk+1), Args: args, Directory: ".", TimeoutSeconds: presetTimeout, ReportFormat: "tap"})
		start = end
	}
	return commands
}

func testRunnerArgs(script string) []string {
	fields := strings.Fields(script)
	for i, field := range fields {
		if strings.Contains(field, "vitest") {
			if i+1 < len(fields) && fields[i+1] == "run" {
				return nil
			}
			return []string{"--run"}
		}
		if strings.Contains(field, "jest") {
			for _, arg := range fields[i+1:] {
				if arg == "--runInBand" {
					return nil
				}
			}
			return []string{"--runInBand"}
		}
	}
	return nil
}

func supportedTestRunner(script string) bool {
	for _, field := range strings.Fields(script) {
		if strings.Contains(field, "vitest") || strings.Contains(field, "jest") {
			return true
		}
	}
	return strings.Contains(script, "node --test") || strings.Contains(script, "node\t--test")
}

func npmCommand(script, directory string, extra []string, index int) Command {
	id := fmt.Sprintf("js-%02d", index)
	args := []string{"node", "/usr/local/lib/node_modules/npm/bin/npm-cli.js", "run", script}
	if len(extra) > 0 {
		args = append(args, "--")
		args = append(args, extra...)
	}
	return Command{ID: id, Args: args, Directory: directory, TimeoutSeconds: presetTimeout, ReportFormat: "exit"}
}

func baseRecipe(name string) Recipe {
	a, _ := Lookup(name)
	return Recipe{Name: name, Version: CurrentVersion, MinProof: a.MinProof, AllowedPaths: slices.Clone(a.AllowedPaths), MinimumTests: presetMinTests, MaxFiles: presetMaxFiles, MaxPatchBytes: presetMaxPatch, MaxTurns: presetMaxTurns, TimeoutSeconds: presetTimeout}
}

func structuralRecipe(a Archetype) Recipe {
	r := baseRecipe(a.Name)
	r.Version = "v1"
	r.MinimumTests = 0
	r.ReviewOnly = a.ReviewOnly
	r.Commands = []Command{{ID: strings.TrimPrefix(a.Validator, "validate-"), Args: []string{"/opt/reforge/tool", a.Validator}, Directory: ".", TimeoutSeconds: presetTimeout, ReportFormat: "exit"}}
	return r
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
		protect := strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.js") || strings.HasSuffix(name, ".spec.js") || strings.HasSuffix(name, ".test.mjs") || strings.HasSuffix(name, ".spec.mjs") || strings.HasSuffix(name, ".test.cjs") || strings.HasSuffix(name, ".spec.cjs") || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".spec.ts") || strings.HasSuffix(name, ".test.tsx") || strings.HasSuffix(name, ".spec.tsx") || (strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"))
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
