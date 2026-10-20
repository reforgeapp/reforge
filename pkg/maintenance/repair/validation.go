package repair

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
	"github.com/reforgeapp/reforge/pkg/source"
)

var ErrValidation = errors.New("frozen validation plan or evidence is incomplete")
var ErrPatch = errors.New("patch changes protected validation, sensitive paths or exceeds recipe limits")

type Plan struct {
	AuthorityHash   string            `json:"authority_hash,omitempty"`
	MaxChangedLines int               `json:"max_changed_lines"`
	Version         int               `json:"version"`
	BaselineSHA     string            `json:"baseline_sha"`
	TargetSHA       string            `json:"target_sha"`
	Image           string            `json:"image"`
	Recipe          recipes.Recipe    `json:"recipe"`
	ProtectedHashes map[string]string `json:"protected_hashes"`
	ForbiddenPaths  []string          `json:"forbidden_paths"`
	Owner           bool              `json:"owner,omitempty"`
	Digest          string            `json:"digest"`
}

func hashBytes(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func planDigest(p Plan) string     { p.Digest = ""; body, _ := json.Marshal(p); return hashBytes(body) }
func Freeze(name, image, baseline, target string, files map[string][]byte, forbidden []string) (Plan, error) {
	return freeze(name, image, baseline, target, files, forbidden, false)
}
func freeze(name, image, baseline, target string, files map[string][]byte, forbidden []string, owner bool) (Plan, error) {
	var p Plan
	if !source.ValidSHA(baseline, "sha1") || !source.ValidSHA(target, "sha1") || !strings.HasPrefix(image, "sha256:") || !source.ValidSHA(strings.TrimPrefix(image, "sha256:"), "sha256") {
		return p, ErrValidation
	}
	recipe, err := recipes.Build(name, files)
	if owner {
		recipe, err = recipes.BuildOwner(name, files)
	}
	if err != nil {
		return p, err
	}
	p = Plan{MaxChangedLines: 500, Version: 1, BaselineSHA: baseline, TargetSHA: target, Image: image, Recipe: recipe, ProtectedHashes: map[string]string{}, ForbiddenPaths: append([]string{}, forbidden...)}
	slices.Sort(p.ForbiddenPaths)
	for _, file := range recipe.ProtectedPaths {
		p.ProtectedHashes[file] = hashBytes(files[file])
	}
	p.Digest = planDigest(p)
	return p, nil
}
func (p Plan) Valid() bool {
	return p.Version == 1 && p.Digest != "" && p.Digest == planDigest(p) && p.MaxChangedLines > 0 && p.Recipe.MaxFiles > 0 && p.Recipe.MaxPatchBytes > 0 && len(p.Recipe.Commands) > 0
}
func protectedPath(file string) bool {
	lower := strings.ToLower(file)
	name := path.Base(lower)
	for _, part := range strings.Split(lower, "/") {
		if strings.HasPrefix(part, ".") || slices.Contains([]string{"test", "tests", "__tests__", "testdata", "fixtures", "scripts", "auth", "authentication", "identity", "secrets", "security", "infra", "deploy", "k8s", "kubernetes"}, part) {
			return true
		}
	}
	return strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "test_") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") || strings.Contains(name, "config") || name == "setup.py" || strings.HasPrefix(name, "conftest") || strings.Contains(name, "coverage")
}

var bypassCode = regexp.MustCompile(`(?i)(os\.Exit\s*\(|sys\.exit\s*\(|process\.exit\s*\(|os\._exit\s*\(|t\.(Skip|SkipNow|Skipf)\s*\(|(?:test|it|describe)\.(skip|only)\s*\(|unittest\.skip|pytest\.skip|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func CheckPatch(p Plan, baseline map[string][]byte, patches []sandbox.Patch) error {
	if !p.Valid() || len(patches) == 0 || len(patches) > p.Recipe.MaxFiles {
		return ErrPatch
	}
	seen := map[string]bool{}
	size := 0
	lines := 0
	for _, patch := range patches {
		if !guest.ValidPath(patch.Path) || !utf8.ValidString(patch.Path) || seen[patch.Path] || patch.Delete || protectedPath(patch.Path) {
			return ErrPatch
		}
		seen[patch.Path] = true
		lines += changedLines(baseline[patch.Path], patch.Content)
		if p.MaxChangedLines > 0 && lines > p.MaxChangedLines {
			return ErrPatch
		}
		size += len(patch.Content)
		if size > p.Recipe.MaxPatchBytes || bytes.IndexByte(patch.Content, 0) >= 0 || !utf8.Valid(patch.Content) {
			return ErrPatch
		}
		if _, protected := p.ProtectedHashes[patch.Path]; protected {
			return ErrPatch
		}
		for _, pattern := range p.ForbiddenPaths {
			if policy.ForbiddenPath(pattern, patch.Path) {
				return ErrPatch
			}
		}
		ext := strings.ToLower(path.Ext(patch.Path))
		allowed := p.Recipe.Name == "go" && ext == ".go" || p.Recipe.Name == "javascript" && slices.Contains([]string{".js", ".mjs", ".cjs"}, ext) || p.Recipe.Name == "python" && ext == ".py"
		if !allowed || bypassCode.Match(patch.Content) && !bytes.Equal(patch.Content, baseline[patch.Path]) {
			return ErrPatch
		}
	}
	return nil
}

type CheckResult struct {
	CommandID    string            `json:"command_id"`
	ExitCode     int               `json:"exit_code"`
	OutputSHA256 string            `json:"output_sha256"`
	Complete     bool              `json:"complete"`
	Cases        map[string]string `json:"cases"`
	Reason       string            `json:"reason"`
	Excerpt      string            `json:"excerpt,omitempty"`
}

var tapCase = regexp.MustCompile(`^\s*(not ok|ok) [0-9]+ - (.+)$`)
var pythonCase = regexp.MustCompile(`^(.+) \.\.\. (ok|FAIL|ERROR|skipped .+)$`)

type checkSummary struct {
	CommandID    string   `json:"command_id"`
	ExitCode     int      `json:"exit_code"`
	Complete     bool     `json:"complete"`
	Pass         int      `json:"pass"`
	Fail         int      `json:"fail"`
	Skip         int      `json:"skip"`
	Other        int      `json:"other"`
	FailureCases []string `json:"failure_cases,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Excerpt      string   `json:"excerpt,omitempty"`
}

func summarizeChecks(results []CheckResult) []byte {
	summary := make([]checkSummary, 0, len(results))
	for _, result := range results {
		check := checkSummary{CommandID: result.CommandID, ExitCode: result.ExitCode, Complete: result.Complete}
		cases := make([]string, 0, len(result.Cases))
		for name, state := range result.Cases {
			switch state {
			case "pass":
				check.Pass++
			case "fail":
				check.Fail++
				cases = append(cases, name)
			case "skip":
				check.Skip++
			default:
				check.Other++
			}
		}
		if check.Fail > 0 {
			slices.Sort(cases)
			if len(cases) > 5 {
				cases = cases[:5]
			}
			for i := range cases {
				cases[i] = clipRunes(cases[i], 200)
			}
			check.FailureCases = cases
		}
		if !result.Complete || result.ExitCode != 0 || check.Fail > 0 {
			check.Reason = clipRunes(result.Reason, 240)
			check.Excerpt = clipRunes(result.Excerpt, 1200)
		}
		summary = append(summary, check)
	}
	body, _ := json.Marshal(summary)
	return body
}

func clipRunes(value string, limit int) string {
	runes := []rune(strings.ToValidUTF8(value, "�"))
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}

func Interpret(command recipes.Command, result sandbox.CommandResult) CheckResult {
	out := CheckResult{CommandID: command.ID, ExitCode: result.ExitCode, OutputSHA256: hashBytes(result.Output), Complete: !result.TimedOut && !result.Truncated, Cases: map[string]string{}}
	if result.ExitCode < 0 || result.ExitCode == 126 || result.ExitCode == 127 {
		out.Complete = false
	}
	if !out.Complete {
		out.Reason = "Command timed out or output was truncated"
		return out
	}
	excerpt := result.Output
	if len(excerpt) > 8192 {
		excerpt = excerpt[len(excerpt)-8192:]
	}
	out.Excerpt = string(excerpt)
	body := string(result.Output)
	for _, failure := range []string{"executable file not found", "Network is unreachable", "Temporary failure in name resolution", "network is disabled", "permission denied", "no required module provides package", "cannot find module", "ModuleNotFoundError"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(failure)) {
			out.Complete = false
			out.Reason = "Execution environment or required dependency unavailable"
		}
	}
	if command.ReportFormat == "exit" {
		return out
	}
	for _, line := range strings.Split(body, "\n") {
		switch command.ReportFormat {
		case "json":
			var event struct{ Action, Package, Test string }
			if json.Unmarshal([]byte(line), &event) != nil || event.Test == "" {
				continue
			}
			if slices.Contains([]string{"pass", "fail", "skip"}, event.Action) {
				out.Cases[event.Package+"/"+event.Test] = event.Action
			}
		case "tap":
			match := tapCase.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			state := "pass"
			if match[1] == "not ok" {
				state = "fail"
			}
			if strings.Contains(strings.ToUpper(match[2]), "# SKIP") || strings.Contains(strings.ToUpper(match[2]), "# TODO") {
				state = "skip"
			}
			out.Cases[fmt.Sprintf("%d:%s", len(out.Cases)+1, match[2])] = state
		case "text":
			match := pythonCase.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			state := "pass"
			if match[2] == "FAIL" || match[2] == "ERROR" {
				state = "fail"
			} else if strings.HasPrefix(match[2], "skipped") {
				state = "skip"
			}
			out.Cases[match[1]] = state
		default:
			out.Complete = false
			out.Reason = "Unsupported validation report format"
		}
	}
	return out
}
func Reproduced(results []CheckResult) bool {
	failed := false
	for _, result := range results {
		if !result.Complete {
			return false
		}
		if result.ExitCode != 0 {
			failed = true
		}
	}
	return len(results) > 0 && failed
}
func Verified(p Plan, baseline, candidate []CheckResult) bool {
	if p.Owner {
		return ownerVerified(p, baseline, candidate)
	}
	if !p.Valid() || len(baseline) != len(p.Recipe.Commands) || len(candidate) != len(baseline) {
		return false
	}
	count := 0
	for i, result := range candidate {
		if !baseline[i].Complete || !result.Complete || result.ExitCode != 0 || result.CommandID != p.Recipe.Commands[i].ID || baseline[i].CommandID != result.CommandID || len(result.Cases) == 0 {
			return false
		}
		for _, state := range result.Cases {
			if state != "pass" {
				return false
			}
			count++
		}
		for name := range baseline[i].Cases {
			if result.Cases[name] != "pass" {
				return false
			}
		}
	}
	return count >= p.Recipe.MinimumTests
}

func secretFile(name string, body []byte) bool {
	lower := strings.ToLower(name)
	base := path.Base(lower)
	for _, part := range strings.Split(lower, "/") {
		if part == "secrets" || part == "credentials" {
			return true
		}
	}
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || bytes.Contains(body, []byte("PRIVATE KEY-----"))
}

func CheckOwnerPatch(p Plan, baseline map[string][]byte, patches []sandbox.Patch) error {
	if !p.Valid() || !p.Owner || len(patches) == 0 || len(patches) > p.Recipe.MaxFiles {
		return ErrPatch
	}
	seen := map[string]bool{}
	for _, patch := range patches {
		_, exists := baseline[patch.Path]
		if !guest.ValidPath(patch.Path) || !utf8.ValidString(patch.Path) || seen[patch.Path] || secretFile(patch.Path, patch.Content) || patch.Delete && (!exists || len(patch.Content) > 0) || p.Recipe.ReadOnly || len(p.Recipe.AllowedPaths) > 0 && !slices.ContainsFunc(p.Recipe.AllowedPaths, func(glob string) bool { return policy.ForbiddenPath(glob, patch.Path) }) {
			return ErrPatch
		}
		seen[patch.Path] = true
		if bytes.IndexByte(patch.Content, 0) >= 0 || !utf8.Valid(patch.Content) {
			return ErrPatch
		}
		for _, pattern := range p.ForbiddenPaths {
			if policy.ForbiddenPath(pattern, patch.Path) {
				return ErrPatch
			}
		}
	}
	if !withinBytes(p, patches) {
		return ErrPatch
	}
	source := slices.DeleteFunc(slices.Clone(patches), func(patch sandbox.Patch) bool { return recipes.GeneratedPath(patch.Path) })
	if p.MaxChangedLines > 0 && PatchLines(baseline, source) > p.MaxChangedLines {
		return ErrPatch
	}
	return nil
}

func ownerVerified(p Plan, baseline, candidate []CheckResult) bool {
	if !p.Valid() || len(candidate) != len(p.Recipe.Commands) || lostPassingCase(baseline, candidate) {
		return false
	}
	for i, result := range candidate {
		if !result.Complete || result.ExitCode != 0 || result.CommandID != p.Recipe.Commands[i].ID {
			return false
		}
		for _, state := range result.Cases {
			if state != "pass" {
				return false
			}
		}
	}
	return true
}

func withinBytes(p Plan, patches []sandbox.Patch) bool {
	source, generated := 0, 0
	for _, patch := range patches {
		if recipes.GeneratedPath(patch.Path) {
			generated += len(patch.Content)
		} else {
			source += len(patch.Content)
		}
	}
	if p.Recipe.MaxGenerated == 0 {
		return source+generated <= p.Recipe.MaxPatchBytes
	}
	return source <= p.Recipe.MaxPatchBytes && generated <= p.Recipe.MaxGenerated
}

func lostPassingCase(baseline, candidate []CheckResult) bool {
	for i, before := range baseline {
		if i >= len(candidate) {
			return true
		}
		after := map[string]bool{}
		for name, state := range candidate[i].Cases {
			after[caseName(name)] = after[caseName(name)] || state == "pass"
		}
		for name, state := range before.Cases {
			if state == "pass" && !after[caseName(name)] {
				return true
			}
		}
	}
	return false
}

func caseName(name string) string {
	if index, rest, ok := strings.Cut(name, ":"); ok && strings.Trim(index, "0123456789") == "" {
		return rest
	}
	return name
}

func changedLines(a, b []byte) int {
	old, next := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	prefix := 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	tail := 0
	for tail < len(old)-prefix && tail < len(next)-prefix && old[len(old)-1-tail] == next[len(next)-1-tail] {
		tail++
	}
	return len(old) + len(next) - 2*prefix - 2*tail
}
