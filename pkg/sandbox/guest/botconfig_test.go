package guest

import (
	"testing"
	"testing/fstest"
)

func TestValidateBotConfig(t *testing.T) {
	grouped := "version: 2\nupdates:\n  - package-ecosystem: gomod\n    directory: /\n    schedule: {interval: weekly}\n    groups: {all: {patterns: ['*']}}\n"
	for name, tc := range map[string]struct {
		files fstest.MapFS
		valid bool
	}{
		"missing":   {fstest.MapFS{}, false},
		"ungrouped": {fstest.MapFS{".github/dependabot.yml": {Data: []byte("version: 2\nupdates:\n  - package-ecosystem: gomod\n    directory: /\n    schedule: {interval: weekly}\n")}}, false},
		"grouped":   {fstest.MapFS{".github/dependabot.yml": {Data: []byte(grouped)}}, true},
		"renovate":  {fstest.MapFS{"renovate.json": {Data: []byte(`{"extends":["config:recommended"]}`)}}, true},
		"broken":    {fstest.MapFS{"renovate.json": {Data: []byte(`{"extends":`)}}, false},
	} {
		if problems := ValidateBotConfig(tc.files); (len(problems) == 0) != tc.valid {
			t.Errorf("%s: %v", name, problems)
		}
	}
}

func TestValidateBootstrap(t *testing.T) {
	workflow := &fstest.MapFile{Data: []byte("on: push\njobs: {test: {runs-on: ubuntu-latest}}\n")}
	for name, tc := range map[string]struct {
		files fstest.MapFS
		valid bool
	}{
		"empty":    {fstest.MapFS{"main.go": {Data: []byte("package main")}}, false},
		"no-tests": {fstest.MapFS{".github/workflows/ci.yml": workflow}, false},
		"broken":   {fstest.MapFS{".github/workflows/ci.yml": {Data: []byte("on: [")}, "main_test.go": {}}, false},
		"complete": {fstest.MapFS{".github/workflows/ci.yml": workflow, "tests/test_app.py": {}}, true},
		"vendored": {fstest.MapFS{".github/workflows/ci.yml": workflow, "node_modules/x/a.test.js": {}}, false},
	} {
		if problems := ValidateBootstrap(tc.files); (len(problems) == 0) != tc.valid {
			t.Errorf("%s: %v", name, problems)
		}
	}
}
