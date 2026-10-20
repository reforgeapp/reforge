package detectors

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	maxFiles = 4096
	maxBytes = 8 << 20
	maxDeps  = 10000
)

type Dependency struct {
	Ecosystem string `json:"ecosystem"`
	Manifest  string `json:"manifest"`
	Name      string `json:"name"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type Result struct {
	Changes  []Dependency
	Complete bool
	Reasons  []string
}

type BotStatus struct {
	Present   bool   `json:"present"`
	AutoMerge string `json:"automerge"`
}

type BotConfig struct {
	Renovate   BotStatus `json:"renovate"`
	Dependabot BotStatus `json:"dependabot"`
}

func Compare(base, candidate map[string][]byte) Result {
	r := Result{Complete: true}
	if len(base) > maxFiles || len(candidate) > maxFiles {
		return incomplete("manifest input exceeds file limit")
	}
	totalBytes := 0
	for _, files := range []map[string][]byte{base, candidate} {
		for _, data := range files {
			totalBytes += len(data)
			if totalBytes > maxBytes {
				return incomplete("manifest input exceeds aggregate byte limit")
			}
		}
	}
	files := map[string]bool{}
	for name := range base {
		files[name] = true
	}
	for name := range candidate {
		files[name] = true
	}
	keys := make([]string, 0, len(files))
	for name := range files {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	old, now := map[string]Dependency{}, map[string]Dependency{}
	for _, name := range keys {
		b, bok := base[name]
		c, cok := candidate[name]
		if bok && len(b) > maxBytes || cok && len(c) > maxBytes {
			r.Reasons = append(r.Reasons, name+": content exceeds byte limit")
			r.Complete = false
			continue
		}
		if bok {
			deps, reasons := parseManifest(name, b)
			for _, reason := range reasons {
				r.Reasons = append(r.Reasons, name+": "+bounded(reason))
			}
			if len(reasons) > 0 {
				r.Complete = false
			}
			for _, d := range deps {
				old[identity(d)] = d
			}
		}
		if cok {
			deps, reasons := parseManifest(name, c)
			for _, reason := range reasons {
				r.Reasons = append(r.Reasons, name+": "+bounded(reason))
			}
			if len(reasons) > 0 {
				r.Complete = false
			}
			for _, d := range deps {
				now[identity(d)] = d
			}
		}
	}
	if len(old)+len(now) > maxDeps {
		return incomplete("dependency input exceeds dependency limit")
	}
	ids := map[string]bool{}
	for id := range old {
		ids[id] = true
	}
	for id := range now {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		a, ao := old[id]
		b, bo := now[id]
		switch {
		case ao && bo && a.From != b.From:
			r.Changes = append(r.Changes, Dependency{Ecosystem: b.Ecosystem, Manifest: b.Manifest, Name: b.Name, From: a.From, To: b.From})
		case ao && !bo:
			r.Changes = append(r.Changes, Dependency{Ecosystem: a.Ecosystem, Manifest: a.Manifest, Name: a.Name, From: a.From})
		case !ao && bo:
			r.Changes = append(r.Changes, Dependency{Ecosystem: b.Ecosystem, Manifest: b.Manifest, Name: b.Name, To: b.From})
		}
	}
	return r
}

func BotConfiguration(files map[string][]byte) BotConfig {
	out := BotConfig{Renovate: BotStatus{AutoMerge: "unknown"}, Dependabot: BotStatus{AutoMerge: "unknown"}}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	renovateSeen := false
	for _, name := range names {
		data := files[name]
		lower := strings.ToLower(filepath.ToSlash(name))
		if lower == "renovate.json5" || strings.HasSuffix(lower, "/renovate.json5") {
			out.Renovate = BotStatus{Present: true, AutoMerge: "unknown"}
			renovateSeen = true
		}
		if lower == ".renovaterc.json" || lower == ".renovaterc.json5" || strings.HasSuffix(lower, "/.renovaterc.json") || strings.HasSuffix(lower, "/.renovaterc.json5") || lower == "renovate.config.js" || lower == "renovate.config.cjs" || lower == "renovate.config.mjs" || strings.HasSuffix(lower, "/renovate.config.js") || strings.HasSuffix(lower, "/renovate.config.cjs") || strings.HasSuffix(lower, "/renovate.config.mjs") {
			out.Renovate = BotStatus{Present: true, AutoMerge: "unknown"}
			renovateSeen = true
		}
		if lower == "renovate.json" || lower == ".renovaterc" || strings.HasSuffix(lower, "/renovate.json") || strings.HasSuffix(lower, "/.renovaterc") {
			status := botStatus(data)
			if renovateSeen {
				status.AutoMerge = "unknown"
			}
			out.Renovate = status
			out.Renovate.Present = true
			renovateSeen = true
		}
		if lower == ".github/dependabot.yml" || lower == ".github/dependabot.yaml" || strings.HasSuffix(lower, "/.github/dependabot.yml") || strings.HasSuffix(lower, "/.github/dependabot.yaml") {
			out.Dependabot = BotStatus{Present: true, AutoMerge: "unknown"}
		}
	}
	return out
}

func botStatus(data []byte) BotStatus {
	if len(data) > maxBytes || len(duplicateJSONKeys(data)) != 0 {
		return BotStatus{AutoMerge: "unknown"}
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return BotStatus{AutoMerge: "unknown"}
	}
	if _, ok := root["extends"]; ok {
		return BotStatus{AutoMerge: "unknown"}
	}
	if _, ok := root["packageRules"]; ok {
		return BotStatus{AutoMerge: "unknown"}
	}
	if raw, ok := root["automerge"]; ok {
		var enabled bool
		if json.Unmarshal(raw, &enabled) != nil {
			return BotStatus{AutoMerge: "unknown"}
		}
		if enabled {
			return BotStatus{AutoMerge: "enabled"}
		}
		return BotStatus{AutoMerge: "disabled"}
	}
	return BotStatus{AutoMerge: "unknown"}
}

func bounded(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 120 {
		return value[:120]
	}
	return value
}

func incomplete(reason string) Result { return Result{Complete: false, Reasons: []string{reason}} }
func identity(d Dependency) string    { return d.Ecosystem + "\x00" + d.Manifest + "\x00" + d.Name }

func parseManifest(name string, data []byte) ([]Dependency, []string) {
	n := filepath.ToSlash(name)
	base := filepath.Base(n)
	switch {
	case base == "go.mod":
		return parseGo(n, data)
	case base == "package.json":
		return parseJSON(n, data)
	case base == "pyproject.toml":
		return parsePyproject(n, data)
	case strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"):
		return parseRequirements(n, data)
	default:
		return nil, nil
	}
}

var goRequire = regexp.MustCompile(`^\s*([^\s]+)\s+([^\s]+)(?:\s+//.*)?$`)

func parseGo(name string, data []byte) ([]Dependency, []string) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []Dependency
	reasons := []string{}
	inBlock := false
	seen := map[string]bool{}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "//") {
			continue
		}
		if strings.HasPrefix(trim, "require (") {
			if trim != "require (" {
				reasons = append(reasons, "malformed require block")
				continue
			}
			inBlock = true
			continue
		}
		if inBlock && trim == ")" {
			inBlock = false
			continue
		}
		if strings.HasPrefix(trim, "require ") {
			trim = strings.TrimSpace(strings.TrimPrefix(trim, "require "))
		} else if !inBlock {
			keyword := strings.Fields(trim)
			if len(keyword) == 0 || keyword[0] == "module" || keyword[0] == "go" {
				continue
			}
			reasons = append(reasons, "unsupported or malformed go.mod directive")
			continue
		}
		m := goRequire.FindStringSubmatch(trim)
		if m == nil || strings.HasPrefix(trim, "//") {
			reasons = append(reasons, "malformed require directive")
			continue
		}
		if seen[m[1]] {
			reasons = append(reasons, "duplicate module "+m[1])
			continue
		}
		seen[m[1]] = true
		out = append(out, Dependency{"go", name, m[1], m[2], ""})
	}
	if inBlock {
		reasons = append(reasons, "unterminated require block")
	}
	return out, reasons
}

func parseJSON(name string, data []byte) ([]Dependency, []string) {
	if reasons := duplicateJSONKeys(data); len(reasons) > 0 {
		return nil, reasons
	}
	var root struct {
		Dependencies map[string]string `json:"dependencies"`
		Dev          map[string]string `json:"devDependencies"`
		Peer         map[string]string `json:"peerDependencies"`
		Optional     map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, []string{"malformed JSON: " + err.Error()}
	}
	var out []Dependency
	seen := map[string]bool{}
	for _, set := range []map[string]string{root.Dependencies, root.Dev, root.Peer, root.Optional} {
		for pkg, version := range set {
			if seen[pkg] {
				return nil, []string{"dependency appears in multiple package.json sets: " + pkg}
			}
			seen[pkg] = true
			out = append(out, Dependency{"npm", name, pkg, version, ""})
		}
	}
	return out, nil
}

func duplicateJSONKeys(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	var reasons []string
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		if d == '{' {
			seen := map[string]bool{}
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return err
				}
				k := key.(string)
				if seen[k] {
					reasons = append(reasons, "duplicate JSON key "+k)
				}
				seen[k] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		}
		if d == '[' {
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		}
		return nil
	}
	if err := walk(); err != nil {
		return []string{"malformed JSON: " + err.Error()}
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return []string{"malformed JSON: trailing content"}
		}
		return []string{"malformed JSON: " + err.Error()}
	}
	return reasons
}

func parsePyproject(name string, data []byte) ([]Dependency, []string) {
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, []string{"malformed TOML: " + err.Error()}
	}
	var out []Dependency
	reasons := []string{}
	if raw, exists := root["project"]; exists {
		if _, valid := raw.(map[string]any); !valid {
			reasons = append(reasons, "project table has ambiguous type")
		}
	}
	if raw, exists := root["tool"]; exists {
		if _, valid := raw.(map[string]any); !valid {
			reasons = append(reasons, "tool table has ambiguous type")
		}
	}
	project := table(root, "project")
	var seen = map[string]bool{}
	deps := project["dependencies"]
	if deps != nil {
		if !isStringSlice(deps) {
			reasons = append(reasons, "project.dependencies has ambiguous type")
		}
	}
	if dynamic, exists := project["dynamic"]; exists {
		if !isStringSlice(dynamic) {
			reasons = append(reasons, "project.dynamic has ambiguous type")
		}
		for _, field := range stringsOf(dynamic) {
			if field == "dependencies" || field == "optional-dependencies" {
				reasons = append(reasons, "project dependencies are dynamic")
			}
		}
	}
	for _, raw := range stringsOf(deps) {
		n, v, ok := pythonRequirement(raw)
		if !ok {
			reasons = append(reasons, "ambiguous PEP 508 dependency "+raw)
			continue
		}
		if seen[n] {
			reasons = append(reasons, "duplicate normalized dependency "+n)
			continue
		}
		seen[n] = true
		out = append(out, Dependency{"python", name, n, v, ""})
	}
	optional := project["optional-dependencies"]
	if optional != nil {
		if _, valid := optional.(map[string]any); !valid {
			reasons = append(reasons, "project.optional-dependencies has ambiguous type")
		}
	}
	for _, values := range stringSlices(optional) {
		for _, raw := range values {
			n, v, ok := pythonRequirement(raw)
			if !ok {
				reasons = append(reasons, "ambiguous PEP 508 dependency "+raw)
				continue
			}
			if seen[n] {
				reasons = append(reasons, "duplicate normalized dependency "+n)
				continue
			}
			seen[n] = true
			out = append(out, Dependency{"python", name, n, v, ""})
		}
	}
	if optionalMap, ok := optional.(map[string]any); ok {
		for group, raw := range optionalMap {
			if !isStringSlice(raw) {
				reasons = append(reasons, "optional dependency group has ambiguous type "+group)
			}
		}
	}
	poetry := table(table(root, "tool"), "poetry")
	if _, exists := poetry["group"]; exists {
		reasons = append(reasons, "Poetry dependency groups are ambiguous")
	}
	poetryDeps := table(poetry, "dependencies")
	if raw, exists := poetry["dependencies"]; exists {
		if _, valid := raw.(map[string]any); !valid {
			reasons = append(reasons, "tool.poetry.dependencies has ambiguous type")
		}
	}
	for pkg, raw := range poetryDeps {
		v, ok := tomlVersion(raw)
		if !ok {
			reasons = append(reasons, "ambiguous Poetry dependency "+pkg)
			continue
		}
		n := normalizePythonName(pkg)
		if seen[n] {
			reasons = append(reasons, "duplicate normalized dependency "+n)
			continue
		}
		seen[n] = true
		out = append(out, Dependency{"python", name, n, v, ""})
	}
	return out, reasons
}
func table(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}
func stringsOf(v any) []string {
	if a, ok := v.([]string); ok {
		return a
	}
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, x := range a {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func isStringSlice(v any) bool {
	if _, ok := v.([]string); ok {
		return true
	}
	items, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}
func stringSlices(v any) map[string][]string {
	if m, ok := v.(map[string]any); ok {
		out := map[string][]string{}
		for k, value := range m {
			out[k] = stringsOf(value)
		}
		return out
	}
	return nil
}
func tomlVersion(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case map[string]any:
		if s, ok := x["version"].(string); ok {
			for key := range x {
				if key != "version" {
					return "", false
				}
			}
			return s, true
		}
	}
	return "", false
}

var pyReq = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)(\[[^]]+\])?(.*)$`)

func pythonRequirement(raw string) (string, string, bool) {
	m := pyReq.FindStringSubmatch(raw)
	if m == nil {
		return "", "", false
	}
	tail := strings.TrimSpace(m[2] + m[3])
	if !validPythonTail(tail) {
		return "", "", false
	}
	return normalizePythonName(m[1]), tail, true
}

var pythonVersions = regexp.MustCompile(`^(?:(?:===|~=|==|!=|<=|>=|<|>)\s*[A-Za-z0-9_.!*+-]+)(?:\s*,\s*(?:===|~=|==|!=|<=|>=|<|>)\s*[A-Za-z0-9_.!*+-]+)*$`)
var pythonExtras = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(?:\s*,\s*[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
var pythonMarker = regexp.MustCompile(`^(?:python_version|python_full_version|os_name|sys_platform|platform_release|platform_system|platform_version|platform_machine|platform_python_implementation|implementation_name|implementation_version|extra)\s*(?:===|~=|==|!=|<=|>=|<|>|not in|in)\s*(?:"[^"\\]*"|'[^'\\]*')$`)

func validPythonTail(tail string) bool {
	if strings.HasPrefix(tail, "[") {
		end := strings.IndexByte(tail, ']')
		if end < 2 || !pythonExtras.MatchString(tail[1:end]) {
			return false
		}
		tail = strings.TrimSpace(tail[end+1:])
	}
	parts := strings.Split(tail, ";")
	if len(parts) > 2 {
		return false
	}
	versions := strings.TrimSpace(parts[0])
	if versions != "" && !pythonVersions.MatchString(versions) {
		return false
	}
	if len(parts) == 2 {
		terms := regexp.MustCompile(`\s+(?:and|or)\s+`).Split(strings.TrimSpace(parts[1]), -1)
		for _, term := range terms {
			if !pythonMarker.MatchString(strings.TrimSpace(term)) {
				return false
			}
		}
	}
	return true
}

func normalizePythonName(name string) string {
	return regexp.MustCompile(`[-_.]+`).ReplaceAllString(strings.ToLower(name), "-")
}

var requirementComment = regexp.MustCompile(`\s+#.*$`)

func parseRequirements(name string, data []byte) ([]Dependency, []string) {
	var out []Dependency
	var reasons []string
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "-") {
			reasons = append(reasons, "unsupported requirement option or inclusion "+bounded(t))
			continue
		}
		line := requirementComment.ReplaceAllString(t, "")
		n, version, valid := pythonRequirement(line)
		if !valid {
			reasons = append(reasons, "ambiguous requirement "+t)
			continue
		}
		if seen[n] {
			reasons = append(reasons, "duplicate requirement "+n)
			continue
		}
		seen[n] = true
		out = append(out, Dependency{"python", name, n, version, ""})
	}
	return out, reasons
}
