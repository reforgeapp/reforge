package guest

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".venv": true, "dist": true, "build": true}

var behaviourCheck = regexp.MustCompile(`(?m)(?:^|\n|&&|;)\s*(?:go\s+test\b|node\s+--test\b|(?:npm|pnpm|yarn)\s+(?:run\s+)?test(?:[:\s]|$)|python(?:3)?\s+-m\s+(?:unittest|pytest)\b|pytest\b)`)
var explicitTestFailure = regexp.MustCompile(`\bthrow\s+(?:new\s+)?(?:Error|AssertionError)\s*\(`)
var trivialAssertion = regexp.MustCompile(`(?i)(?:assert\s+(?:true|1\s*==\s*1)|assert\.(?:ok|equal|strictEqual)\(\s*(?:true|1\s*,\s*1)\s*\)|expect\(\s*(?:true|1)\s*\)\.(?:toBe|toEqual)\(\s*(?:true|1)\s*\)|console\.assert\s*\()`)

func behaviouralTest(name string, body []byte) bool {
	text := string(body)
	if strings.TrimSpace(text) == "" || trivialAssertion.Match(body) {
		return false
	}
	if strings.HasSuffix(name, "_test.go") {
		file, err := parser.ParseFile(token.NewFileSet(), name, body, 0)
		if err != nil {
			return false
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Body == nil {
				continue
			}
			assertion, application := false, false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					application = application || !strings.HasPrefix(fun.Name, "Test") && !strings.HasPrefix(fun.Name, "make") && !strings.HasPrefix(fun.Name, "len")
				case *ast.SelectorExpr:
					if receiver, ok := fun.X.(*ast.Ident); ok {
						assertion = assertion || receiver.Name == "t" && (strings.HasPrefix(fun.Sel.Name, "Error") || strings.HasPrefix(fun.Sel.Name, "Fatal")) || receiver.Name == "assert" || receiver.Name == "require"
						application = application || receiver.Name != "t" && receiver.Name != "assert" && receiver.Name != "require"
					}
				}
				return true
			})
			if assertion && application {
				return true
			}
		}
		return false
	}
	if strings.HasSuffix(name, ".py") {
		return (strings.Contains(text, "assert ") || strings.Contains(text, "self.assert")) && (strings.Contains(text, "from ") || strings.Contains(text, "import "))
	}
	return (strings.Contains(text, "assert") || strings.Contains(text, "expect(") || explicitTestFailure.MatchString(text)) && (strings.Contains(text, "'./") || strings.Contains(text, "\"./") || strings.Contains(text, "'../") || strings.Contains(text, "\"../") || strings.Contains(text, "page.goto("))
}

func workflowRunsTests(doc map[string]any) bool {
	jobs, ok := doc["jobs"].(map[string]any)
	if ok {
		triggered := false
		switch on := doc["on"].(type) {
		case string:
			triggered = on == "push" || on == "pull_request"
		case []any:
			for _, event := range on {
				triggered = triggered || event == "push" || event == "pull_request"
			}
		case map[string]any:
			for _, event := range []string{"push", "pull_request"} {
				value, exists := on[event]
				if !exists {
					continue
				}
				if filters, ok := value.(map[string]any); ok && (filters["paths"] != nil || filters["paths-ignore"] != nil) {
					continue
				}
				triggered = true
			}
		}
		if !triggered {
			return false
		}
	}
	if !ok {
		jobs = doc
	}
	for _, value := range jobs {
		job, ok := value.(map[string]any)
		if !ok || job["if"] != nil || job["continue-on-error"] == true || job["allow_failure"] == true {
			continue
		}
		if steps, ok := job["steps"].([]any); ok {
			for _, value := range steps {
				step, ok := value.(map[string]any)
				if !ok || step["if"] != nil || step["continue-on-error"] == true {
					continue
				}
				if run, ok := step["run"].(string); ok && behaviourCheck.MatchString(run) && !strings.Contains(run, "||") {
					return true
				}
			}
		}
		if script, ok := job["script"].([]any); ok {
			for _, value := range script {
				if run, ok := value.(string); ok && behaviourCheck.MatchString(run) && !strings.Contains(run, "||") {
					return true
				}
			}
		}
	}
	return false
}

func CIPath(file string) bool {
	lower := strings.ToLower(file)
	ext := path.Ext(lower)
	workflow := (strings.HasPrefix(lower, ".github/workflows/") || strings.HasPrefix(lower, ".gitea/workflows/") || strings.HasPrefix(lower, ".forgejo/workflows/")) && (ext == ".yml" || ext == ".yaml")
	return workflow || lower == ".gitlab-ci.yml"
}

func NodeTestPath(file string) bool {
	name := strings.ToLower(path.Base(file))
	ext := path.Ext(name)
	return (ext == ".js" || ext == ".mjs" || ext == ".cjs") && (name == "test"+ext || strings.HasSuffix(name, ".test"+ext) || strings.HasSuffix(name, ".spec"+ext))
}

func TestPath(file string) bool {
	lower := strings.ToLower(file)
	name := path.Base(lower)
	for _, part := range strings.Split(path.Dir(lower), "/") {
		if part == "tests" || part == "__tests__" {
			return true
		}
	}
	return NodeTestPath(file) || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") || strings.HasSuffix(name, "_test.py") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.")
}

func ValidateBootstrap(root fs.FS) []string {
	problems := []string{}
	ci, functionalCI, tests, files := false, false, false, 0
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if files++; files > 20000 {
			return errors.New("too many files")
		}
		if TestPath(name) {
			body, err := fs.ReadFile(root, name)
			if err != nil {
				return err
			}
			if behaviouralTest(name, body) {
				tests = true
			}
		}
		if !CIPath(name) {
			return nil
		}
		ci = true
		body, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		var doc map[string]any
		if err := yaml.Unmarshal(body, &doc); err != nil || len(doc) == 0 {
			problems = append(problems, name+": not a YAML mapping")
		} else if workflowRunsTests(doc) {
			functionalCI = true
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	if !ci {
		problems = append(problems, "no CI workflow")
	} else if !functionalCI {
		problems = append(problems, "no required behavioural test command in CI")
	}
	if !tests {
		problems = append(problems, "no nonempty behavioural tests")
	}
	return problems
}

var markdownLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)

func BrokenLinks(name string, body []byte, exists func(string) bool) []string {
	broken := []string{}
	for _, match := range markdownLink.FindAllStringSubmatch(string(body), -1) {
		target, _, _ := strings.Cut(match[1], "#")
		if target == "" || strings.Contains(target, ":") || strings.HasPrefix(target, "/") {
			continue
		}
		if !exists(path.Join(path.Dir(name), target)) {
			broken = append(broken, match[1])
		}
	}
	return broken
}

func ValidateDocs(root fs.FS) []string {
	problems := []string{}
	files := 0
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if strings.ToLower(path.Ext(name)) != ".md" {
			return nil
		}
		if files++; files > 5000 {
			return errors.New("too many documents")
		}
		body, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		for _, link := range BrokenLinks(name, body, func(file string) bool { _, err := fs.Stat(root, file); return err == nil }) {
			if len(problems) < 100 {
				problems = append(problems, name+": broken link "+link)
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	return problems
}
