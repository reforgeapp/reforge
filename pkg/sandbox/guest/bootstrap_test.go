package guest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapExecutesBuildAndBehaviour(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	files := map[string]string{
		"go.mod":                  "module example.test/bootstrap\n\ngo 1.22\n",
		"add.go":                  "package bootstrap\nfunc Add(a, b int) int { return a + b }\n",
		"add_test.go":             "package bootstrap\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if got := Add(2, 3); got != 5 { t.Fatalf(\"Add = %d\", got) } }\n",
		".gitea/workflows/ci.yml": "on: [push, pull_request]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go build ./...\n      - run: go test ./...\n",
	}
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var logs bytes.Buffer
	if !RunBootstrap(context.Background(), os.DirFS("."), &logs) || !strings.Contains(logs.String(), `"builds":1`) || !strings.Contains(logs.String(), "TestAdd") {
		t.Fatalf("valid bootstrap rejected: %s", logs.String())
	}
	for name, body := range map[string]string{
		"empty":   "package bootstrap\n",
		"skipped": "package bootstrap\nimport \"testing\"\nfunc TestAdd(t *testing.T) { t.Skip(\"later\"); if Add(2,3)!=5 { t.Fatal(\"wrong\") } }\n",
		"failed":  "package bootstrap\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if Add(2,3)!=6 { t.Fatal(\"wrong\") } }\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile("add_test.go", []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			logs.Reset()
			if RunBootstrap(context.Background(), os.DirFS("."), &logs) {
				t.Fatalf("invalid bootstrap verified: %s", logs.String())
			}
		})
	}
	t.Run("node", func(t *testing.T) {
		t.Chdir(t.TempDir())
		files := map[string]string{
			"package.json":            `{"type":"module","scripts":{"test":"node --test"}}`,
			"add.js":                  "export const add = (a,b) => a+b;\n",
			"test.js":                 "import test from 'node:test'; import { add } from './add.js'; test('adds inputs',()=>{ if (add(2,3) !== 5) throw new Error('wrong sum'); });\n",
			".gitea/workflows/ci.yml": "on: [push, pull_request]\njobs: {test: {runs-on: ubuntu-latest, steps: [{run: npm test}]}}\n",
		}
		for name, body := range files {
			if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
		var logs bytes.Buffer
		if !RunBootstrap(context.Background(), os.DirFS("."), &logs) || !strings.Contains(logs.String(), "adds inputs") {
			t.Fatalf("Node bootstrap failed: %s", logs.String())
		}
		if err := os.WriteFile("add.js", []byte("export const add = (a,b) => a-b;\n"), 0600); err != nil {
			t.Fatal(err)
		}
		logs.Reset()
		if RunBootstrap(context.Background(), os.DirFS("."), &logs) || !strings.Contains(logs.String(), "not ok") {
			t.Fatal("throwing behavioural test failed to detect broken application")
		}
		if err := os.WriteFile("test.js", []byte("import test from 'node:test'; import { add } from './add.js'; test('adds inputs',()=>console.assert(add(2,3) === 5));\n"), 0600); err != nil {
			t.Fatal(err)
		}
		logs.Reset()
		if RunBootstrap(context.Background(), os.DirFS("."), &logs) {
			t.Fatal("non-failing console assertion verified broken application")
		}
	})
}
