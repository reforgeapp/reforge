package guest

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".venv": true, "dist": true, "build": true}

func CIPath(file string) bool {
	lower := strings.ToLower(file)
	ext := path.Ext(lower)
	workflow := (strings.HasPrefix(lower, ".github/workflows/") || strings.HasPrefix(lower, ".gitea/workflows/") || strings.HasPrefix(lower, ".forgejo/workflows/")) && (ext == ".yml" || ext == ".yaml")
	return workflow || lower == ".gitlab-ci.yml"
}

func TestPath(file string) bool {
	lower := strings.ToLower(file)
	name := path.Base(lower)
	for _, part := range strings.Split(path.Dir(lower), "/") {
		if part == "tests" || part == "__tests__" {
			return true
		}
	}
	return strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") || strings.HasSuffix(name, "_test.py") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.")
}

func ValidateBootstrap(root fs.FS) []string {
	problems := []string{}
	ci, tests, files := false, false, 0
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
		tests = tests || TestPath(name)
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
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	if !ci {
		problems = append(problems, "no CI workflow")
	}
	if !tests {
		problems = append(problems, "no tests")
	}
	return problems
}

var markdownLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)

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
		for _, match := range markdownLink.FindAllStringSubmatch(string(body), -1) {
			target, _, _ := strings.Cut(match[1], "#")
			if target == "" || strings.Contains(target, ":") || strings.HasPrefix(target, "/") {
				continue
			}
			if _, err := fs.Stat(root, path.Join(path.Dir(name), target)); err != nil && len(problems) < 100 {
				problems = append(problems, name+": broken link "+match[1])
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	return problems
}
