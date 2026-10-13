package guest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

type BootstrapReport struct {
	Cases    map[string]string `json:"cases"`
	Builds   int               `json:"builds"`
	Complete bool              `json:"complete"`
	Problems []string          `json:"problems"`
}

type bootstrapCommand struct {
	args              []string
	directory, format string
	build             bool
}

var bootstrapTAP = regexp.MustCompile(`^\s*(not ok|ok) [0-9]+ - (.+)$`)
var bootstrapPython = regexp.MustCompile(`^(.+) \.\.\. (ok|FAIL|ERROR|skipped .+)$`)

func bootstrapCases(format, prefix string, body []byte, cases map[string]string) {
	for _, line := range strings.Split(string(body), "\n") {
		switch format {
		case "go":
			var event struct{ Action, Package, Test string }
			if json.Unmarshal([]byte(line), &event) == nil && event.Test != "" && slices.Contains([]string{"pass", "fail", "skip"}, event.Action) {
				cases[prefix+"/"+event.Package+"/"+event.Test] = event.Action
			}
		case "tap":
			match := bootstrapTAP.FindStringSubmatch(line)
			if match != nil {
				if NodeFileSuite(match[2]) {
					continue
				}
				state := "pass"
				if match[1] == "not ok" {
					state = "fail"
				}
				if strings.Contains(strings.ToUpper(line), "# SKIP") || strings.Contains(strings.ToUpper(line), "# TODO") {
					state = "skip"
				}
				cases[prefix+"/"+match[2]] = state
			}
		case "python":
			match := bootstrapPython.FindStringSubmatch(line)
			if match != nil {
				state := "pass"
				if match[2] == "FAIL" || match[2] == "ERROR" {
					state = "fail"
				}
				if strings.HasPrefix(match[2], "skipped") {
					state = "skip"
				}
				cases[prefix+"/"+match[1]] = state
			}
		}
	}
}

type bootstrapOutput struct {
	bytes.Buffer
	truncated bool
}

func (b *bootstrapOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > 256<<10 {
		b.truncated = true
		p = p[:max(0, (256<<10)-b.Len())]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func RunBootstrap(ctx context.Context, root fs.FS, output io.Writer) bool {
	report := BootstrapReport{Cases: map[string]string{}, Complete: true, Problems: ValidateBootstrap(root)}
	commands := []bootstrapCommand{}
	paths := []string{}
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDirs[entry.Name()] || entry.Name() == "__pycache__" {
				return fs.SkipDir
			}
			return nil
		}
		paths = append(paths, name)
		if len(paths) > 20000 {
			return fmt.Errorf("too many files")
		}
		return nil
	})
	if err != nil {
		report.Problems = append(report.Problems, err.Error())
	}
	python := false
	for _, name := range paths {
		directory := path.Dir(name)
		switch {
		case path.Base(name) == "go.mod":
			commands = append(commands, bootstrapCommand{[]string{"go", "build", "./..."}, directory, "", true}, bootstrapCommand{[]string{"go", "test", "-json", "-count=1", "./..."}, directory, "go", false})
		case path.Base(name) == "package.json":
			body, err := fs.ReadFile(root, name)
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err != nil || json.Unmarshal(body, &pkg) != nil {
				report.Problems = append(report.Problems, name+": invalid package manifest")
				continue
			}
			if strings.TrimSpace(pkg.Scripts["build"]) != "" {
				commands = append(commands, bootstrapCommand{[]string{"npm", "run", "build"}, directory, "", true})
			} else {
				for _, file := range paths {
					if strings.HasPrefix(file, directory+"/") || directory == "." {
						if slices.Contains([]string{".js", ".mjs", ".cjs"}, path.Ext(file)) && !TestPath(file) {
							commands = append(commands, bootstrapCommand{[]string{"node", "--check", file}, ".", "", true})
						}
					}
				}
			}
			for _, script := range []string{"lint", "typecheck", "test:unit", "test"} {
				body := strings.TrimSpace(pkg.Scripts[script])
				if body == "" {
					continue
				}
				format := ""
				args := []string{"npm", "run", script}
				if strings.HasPrefix(body, "node --test") && !strings.ContainsAny(body, ";&|") {
					format = "tap"
				}
				if script == "test" || script == "test:unit" {
					if format == "" {
						report.Problems = append(report.Problems, name+": use node --test for independently counted bootstrap tests")
					}
				}
				commands = append(commands, bootstrapCommand{args, directory, format, false})
			}
		case strings.HasSuffix(name, ".py"):
			python = true
		}
	}
	if python {
		commands = append(commands, bootstrapCommand{[]string{"python3", "-m", "compileall", "-q", "."}, ".", "", true}, bootstrapCommand{[]string{"python3", "-m", "unittest", "discover", "-v"}, ".", "python", false})
	}
	if len(commands) == 0 || len(commands) > 64 {
		report.Problems = append(report.Problems, "no bounded supported build/test plan")
		commands = nil
	}
	for i, command := range commands {
		step, cancel := context.WithTimeout(ctx, 120*time.Second)
		cmd := exec.CommandContext(step, command.args[0], command.args[1:]...)
		cmd.WaitDelay = 2 * time.Second
		cmd.Dir = command.directory
		if command.format == "tap" {
			cmd.Env = append(os.Environ(), "NODE_OPTIONS="+strings.TrimSpace(os.Getenv("NODE_OPTIONS")+" --test-reporter=tap --test-isolation=none"))
		}
		var logs bootstrapOutput
		cmd.Stdout, cmd.Stderr = &logs, &logs
		err := cmd.Run()
		cancel()
		fmt.Fprintf(output, "%s: %s\n", command.directory, strings.Join(command.args, " "))
		_, _ = output.Write(logs.Bytes())
		if logs.truncated {
			report.Complete = false
			report.Problems = append(report.Problems, "validation output exceeded limit")
		}
		bootstrapCases(command.format, command.directory+"/"+strings.Join(command.args, " "), logs.Bytes(), report.Cases)
		if command.format != "" {
			prefix := command.directory + "/" + strings.Join(command.args, " ") + "/"
			count := 0
			for name, state := range report.Cases {
				if strings.HasPrefix(name, prefix) && state == "pass" {
					count++
				}
			}
			if count == 0 {
				report.Problems = append(report.Problems, "no passing test executed for "+strings.Join(command.args, " "))
			}
		}
		if err != nil {
			report.Problems = append(report.Problems, fmt.Sprintf("check %d failed: %s", i+1, err))
		} else if command.build {
			report.Builds++
		}
	}
	if report.Builds == 0 {
		report.Problems = append(report.Problems, "no successful build")
	}
	passed := 0
	for _, state := range report.Cases {
		if state == "pass" {
			passed++
		} else if state == "fail" {
			report.Problems = append(report.Problems, "behavioural test failed")
		}
	}
	if passed == 0 {
		report.Problems = append(report.Problems, "no behavioural test executed successfully")
	}
	_, _ = fmt.Fprintln(output)
	_ = json.NewEncoder(output).Encode(map[string]BootstrapReport{"bootstrap": report})
	return report.Complete && len(report.Problems) == 0
}
