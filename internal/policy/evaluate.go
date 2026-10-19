package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/reforgeapp/reforge/internal/domain"
)

func Parse(data []byte) (Policy, error) {
	var p Policy
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, err
	}
	if d.Decode(new(any)) != io.EOF {
		return p, fmt.Errorf("one policy document required")
	}
	return p, Validate(p)
}

func validAction(a Action) bool {
	return a == Read || a == Repair || a == Publish || a == Merge || a == Deploy || a == Recover
}

func Validate(p Policy) error {
	if p.Schema != "maintenance/v1" {
		return fmt.Errorf("unsupported policy schema")
	}
	if len(p.ForbiddenPaths) > 64 || len(p.Required) > 128 || len(p.Deny) > 6 {
		return fmt.Errorf("policy exceeds rule limit")
	}
	for _, a := range p.Deny {
		if !validAction(a) {
			return fmt.Errorf("invalid denied action")
		}
	}
	for _, proof := range p.ReviewProofs {
		if proof != "tests" && proof != "structural" {
			return fmt.Errorf("invalid review proof")
		}
	}
	for _, n := range limitValues(p.Limits) {
		if n != nil && *n < 0 {
			return fmt.Errorf("negative ceiling")
		}
	}
	if p.MaxEvidenceAgeSeconds != nil && (*p.MaxEvidenceAgeSeconds < 1 || *p.MaxEvidenceAgeSeconds > 86400) {
		return fmt.Errorf("invalid evidence age")
	}
	for _, values := range listValues(p.Allow) {
		if len(values) > 200 {
			return fmt.Errorf("allowlist exceeds limit")
		}
		for _, v := range values {
			if strings.TrimSpace(v) == "" || len(v) > 256 {
				return fmt.Errorf("empty allowlist value")
			}
		}
	}
	for _, pattern := range p.ForbiddenPaths {
		if pattern == "" || len(pattern) > 256 || strings.Count(pattern, "/") > 31 || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") || strings.Contains(pattern, "..") {
			return fmt.Errorf("invalid forbidden path")
		}
		if _, err := path.Match(strings.ReplaceAll(pattern, "**", "*"), ""); err != nil {
			return fmt.Errorf("invalid forbidden pattern")
		}
	}
	seen := map[string]bool{}
	for _, r := range p.Required {
		key := requirementKey(r.ID, r.Identity)
		if r.ID == "" || strings.ContainsRune(r.ID, 0) || strings.ContainsRune(r.Identity, 0) || len(r.ID) > 128 || len(r.Identity) > 256 || seen[key] || r.Approvals < 0 || r.Approvals > 1000 || len(r.Actions) == 0 || len(r.Actions) > 6 {
			return fmt.Errorf("invalid requirement")
		}
		seen[key] = true
		for _, a := range r.Actions {
			if !validAction(a) {
				return fmt.Errorf("invalid requirement action")
			}
		}
	}
	return nil
}

func canonical(p Policy) Policy {
	p.Limits = Limits{copyNumber(p.Limits.Budget), copyNumber(p.Limits.Concurrency), copyNumber(p.Limits.Attempts), copyNumber(p.Limits.ChangedFiles), copyNumber(p.Limits.ChangedLines), copyNumber(p.Limits.OpenChanges)}
	p.MaxEvidenceAgeSeconds = copyNumber(p.MaxEvidenceAgeSeconds)
	p.Allow = Lists{sorted(p.Allow.Recipes), sorted(p.Allow.Models), sorted(p.Allow.Routes), sorted(p.Allow.MergeMethods), sorted(p.Allow.Environments), sorted(p.Allow.Workflows)}
	p.ForbiddenPaths = sorted(p.ForbiddenPaths)
	p.ReviewProofs = sorted(p.ReviewProofs)
	p.Deny = append([]Action(nil), p.Deny...)
	sort.Slice(p.Deny, func(i, j int) bool { return p.Deny[i] < p.Deny[j] })
	p.Required = append([]Requirement(nil), p.Required...)
	for i := range p.Required {
		p.Required[i].Actions = append([]Action(nil), p.Required[i].Actions...)
		sort.Slice(p.Required[i].Actions, func(a, b int) bool { return p.Required[i].Actions[a] < p.Required[i].Actions[b] })
	}
	sort.Slice(p.Required, func(i, j int) bool {
		return requirementKey(p.Required[i].ID, p.Required[i].Identity) < requirementKey(p.Required[j].ID, p.Required[j].Identity)
	})
	return p
}

func requirementKey(id, identity string) string { return id + "\x00" + identity }

func copyNumber(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func sorted(v []string) []string {
	if v == nil {
		return nil
	}
	out := append([]string{}, v...)
	sort.Strings(out)
	return out
}
func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func applies(v []Action, a Action) bool {
	for _, x := range v {
		if x == a {
			return true
		}
	}
	return false
}
func listValues(l Lists) [][]string {
	return [][]string{l.Recipes, l.Models, l.Routes, l.MergeMethods, l.Environments, l.Workflows}
}
func limitValues(l Limits) []*int64 {
	return []*int64{l.Budget, l.Concurrency, l.Attempts, l.ChangedFiles, l.ChangedLines, l.OpenChanges}
}
func intersect(a, b []string) []string {
	if a == nil {
		return sorted(b)
	}
	if b == nil {
		return sorted(a)
	}
	o := []string{}
	for _, v := range a {
		if contains(b, v) && !contains(o, v) {
			o = append(o, v)
		}
	}
	return sorted(o)
}
func minimum(a, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil || *a < *b {
		return a
	}
	return b
}

func Resolve(layers []Layer, repositoryID, primaryTeamID string, paused bool) Resolved {
	r := Resolved{RepositoryID: repositoryID, PrimaryTeamID: primaryTeamID, Paused: paused, ScopePaused: paused, Layers: append([]Layer{}, layers...), Policy: Policy{Schema: "maintenance/v1"}, Problems: []string{}, MissingDefaults: []string{}}
	sort.Slice(r.Layers, func(i, j int) bool {
		a, b := r.Layers[i].Scope, r.Layers[j].Scope
		return a.Kind+":"+a.ID < b.Kind+":"+b.ID
	})
	org, primary := false, false
	var orgDefaults, teamDefaults, repoDefaults Defaults
	missingModel, missingRoute, missingBranch := false, false, false
	seen := map[Scope]bool{}
	requirements := map[string]Requirement{}
	for i := range r.Layers {
		l := &r.Layers[i]
		l.Policy = canonical(l.Policy)
		if seen[l.Scope] {
			r.Problems = append(r.Problems, "duplicate policy scope")
		}
		seen[l.Scope] = true
		if err := Validate(l.Policy); err != nil {
			r.Problems = append(r.Problems, l.Scope.Kind+":"+l.Scope.ID+": invalid policy")
			continue
		}
		switch l.Scope.Kind {
		case "deployment":
		case "organisation":
			org = true
			orgDefaults = l.Policy.Defaults
		case "team":
			if l.Scope.ID == primaryTeamID {
				primary = true
				teamDefaults = l.Policy.Defaults
			} else if primaryTeamID == "" {
				missingModel = missingModel || l.Policy.Defaults.Model != ""
				missingRoute = missingRoute || l.Policy.Defaults.Route != ""
				missingBranch = missingBranch || l.Policy.Defaults.BranchPrefix != ""
			}
		case "repository":
			if l.Scope.ID != repositoryID {
				r.Problems = append(r.Problems, "repository policy mismatch")
			}
			repoDefaults = l.Policy.Defaults
		default:
			r.Problems = append(r.Problems, "invalid policy scope")
		}
		p := &r.Policy
		q := l.Policy
		p.Allow = Lists{intersect(p.Allow.Recipes, q.Allow.Recipes), intersect(p.Allow.Models, q.Allow.Models), intersect(p.Allow.Routes, q.Allow.Routes), intersect(p.Allow.MergeMethods, q.Allow.MergeMethods), intersect(p.Allow.Environments, q.Allow.Environments), intersect(p.Allow.Workflows, q.Allow.Workflows)}
		p.Limits = Limits{minimum(p.Limits.Budget, q.Limits.Budget), minimum(p.Limits.Concurrency, q.Limits.Concurrency), minimum(p.Limits.Attempts, q.Limits.Attempts), minimum(p.Limits.ChangedFiles, q.Limits.ChangedFiles), minimum(p.Limits.ChangedLines, q.Limits.ChangedLines), minimum(p.Limits.OpenChanges, q.Limits.OpenChanges)}
		p.MaxEvidenceAgeSeconds = minimum(p.MaxEvidenceAgeSeconds, q.MaxEvidenceAgeSeconds)
		for _, a := range q.Deny {
			if !applies(p.Deny, a) {
				p.Deny = append(p.Deny, a)
			}
		}
		for _, pattern := range q.ForbiddenPaths {
			if !contains(p.ForbiddenPaths, pattern) {
				p.ForbiddenPaths = append(p.ForbiddenPaths, pattern)
			}
		}
		for _, proof := range q.ReviewProofs {
			if !contains(p.ReviewProofs, proof) {
				p.ReviewProofs = append(p.ReviewProofs, proof)
			}
		}
		for _, req := range q.Required {
			for _, action := range req.Actions {
				key := req.ID + "\x00" + req.Identity + "\x00" + string(action)
				old, ok := requirements[key]
				if !ok || req.Approvals > old.Approvals {
					requirements[key] = Requirement{ID: req.ID, Identity: req.Identity, Actions: []Action{action}, Approvals: req.Approvals}
				}
			}
		}
		r.Paused = r.Paused || q.Paused
	}
	keys := []string{}
	for key := range requirements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		r.Policy.Required = append(r.Policy.Required, requirements[key])
	}
	r.Policy.ForbiddenPaths = sorted(r.Policy.ForbiddenPaths)
	r.Policy.ReviewProofs = sorted(r.Policy.ReviewProofs)
	sort.Slice(r.Policy.Deny, func(i, j int) bool { return r.Policy.Deny[i] < r.Policy.Deny[j] })
	r.Policy.Paused = r.Paused
	if !org {
		r.Problems = append(r.Problems, "organisation policy missing")
	}
	if primaryTeamID != "" && !primary {
		r.Problems = append(r.Problems, "primary team policy missing")
	}
	r.Policy.Defaults = orgDefaults
	for _, d := range []Defaults{teamDefaults, repoDefaults} {
		if d.Model != "" {
			r.Policy.Defaults.Model = d.Model
		}
		if d.Route != "" {
			r.Policy.Defaults.Route = d.Route
		}
		if d.BranchPrefix != "" {
			r.Policy.Defaults.BranchPrefix = d.BranchPrefix
		}
	}
	if missingModel && repoDefaults.Model == "" {
		r.MissingDefaults = append(r.MissingDefaults, "model")
	}
	if missingRoute && repoDefaults.Route == "" {
		r.MissingDefaults = append(r.MissingDefaults, "route")
	}
	if missingBranch && repoDefaults.BranchPrefix == "" {
		r.MissingDefaults = append(r.MissingDefaults, "branch_prefix")
	}
	r.Hash = hash(r)
	return r
}

func forbidden(pattern, file string) bool {
	p, f := strings.Split(pattern, "/"), strings.Split(file, "/")
	failed := map[[2]int]bool{}
	var match func(int, int) bool
	match = func(i, j int) bool {
		key := [2]int{i, j}
		if failed[key] {
			return false
		}
		failed[key] = true
		if i == len(p) {
			return j == len(f)
		}
		if p[i] == "**" {
			return match(i+1, j) || (j < len(f) && match(i, j+1))
		}
		if j == len(f) {
			return false
		}
		ok, _ := path.Match(p[i], f[j])
		return ok && match(i+1, j+1)
	}
	return match(0, 0)
}

func Evaluate(r Resolved, in Input) Result {
	out := Result{Decision: domain.Decision{Outcome: "allow", PolicyHash: r.Hash, Blockers: []string{}, RequiredActions: []string{}, Rules: []string{}}, Bindings: r.Layers, EvidenceReferences: []string{}, StartingPolicyHash: in.StartingPolicyHash}
	block := func(state, rule, msg string) {
		if state == "deny" || out.Outcome == "allow" {
			out.Outcome = state
		}
		out.Blockers = append(out.Blockers, msg)
		out.Rules = append(out.Rules, rule)
		out.RequiredActions = append(out.RequiredActions, rule)
	}
	if in.Stage != "" && (in.Stage != "deployment_admission" || in.Action != Deploy && in.Action != Recover) {
		block("deny", "stage", "unknown policy stage")
		return out
	}
	if !validAction(in.Action) {
		block("deny", "action", "unknown action")
		return out
	}
	if in.Action == Read {
		return out
	}
	if len(in.Paths) > 200 || len(in.Evidence) > 256 || len(r.Layers) > 202 {
		block("deny", "input", "evaluation exceeds input limit")
		return out
	}
	for _, file := range in.Paths {
		if len(file) > 1024 || strings.Count(file, "/") > 127 {
			block("deny", "path", "candidate path exceeds limit")
			return out
		}
	}
	for _, l := range r.Layers {
		if Validate(l.Policy) != nil {
			block("unknown", "policy", "invalid policy")
			return out
		}
	}
	for _, p := range r.Problems {
		block("unknown", "policy", p)
	}
	if r.Paused || len(in.PausedScopes) > 0 {
		block("deny", "pause", "applicable scope paused")
	}
	if r.Hash == "" || in.Current.PolicyHash != r.Hash {
		block("unknown", "policy.current", "current policy binding missing or stale")
	}
	if in.Now.IsZero() {
		block("unknown", "time", "evaluation time missing")
	}
	if in.Model == "" && in.Action == Repair {
		if contains(r.MissingDefaults, "model") {
			block("unknown", "defaults.model", "primary team required for model default")
		}
		in.Model = r.Policy.Defaults.Model
	}
	if in.Route == "" && in.Action == Repair {
		if contains(r.MissingDefaults, "route") {
			block("unknown", "defaults.route", "primary team required for route default")
		}
		in.Route = r.Policy.Defaults.Route
	}
	if in.Action == Publish && contains(r.MissingDefaults, "branch_prefix") {
		block("unknown", "defaults.branch_prefix", "primary team required for branch default")
	}
	values := []string{in.Recipe, in.Model, in.Route, in.MergeMethod, in.Environment, in.Workflow}
	names := []string{"recipe", "model", "route", "merge_method", "environment", "workflow"}
	needed := []bool{in.Action == Repair || in.Action == Publish, in.Action == Repair, in.Action == Repair, in.Action == Merge, in.Action == Deploy || in.Action == Recover, in.Action == Deploy || in.Action == Recover}
	for i, list := range listValues(r.Policy.Allow) {
		if needed[i] && values[i] == "" {
			block("unknown", "allow."+names[i], names[i]+" missing")
		}
		if (needed[i] || values[i] != "") && list != nil && !contains(list, values[i]) {
			block("deny", "allow."+names[i], names[i]+" disallowed")
		}
	}
	for _, file := range in.Paths {
		if file == "" || path.Clean(file) != file || strings.HasPrefix(file, "/") || file == ".." || strings.HasPrefix(file, "../") || strings.Contains(file, "\\") {
			block("deny", "path", "invalid candidate path")
		}
	}
	usage := limitValues(in.Usage)
	limitNames := []string{"budget", "concurrency", "attempts", "changed_files", "changed_lines", "open_changes"}
	for i, limit := range limitValues(r.Policy.Limits) {
		if usage[i] != nil && *usage[i] < 0 {
			block("deny", "limit."+limitNames[i], "negative resource usage")
		}
		if limit != nil {
			if usage[i] == nil {
				block("unknown", "limit."+limitNames[i], "resource usage missing: "+limitNames[i])
			} else if *usage[i] > *limit {
				block("deny", "limit."+limitNames[i], "resource ceiling exceeded: "+limitNames[i])
			}
		}
	}
	required := map[string]int{}
	for _, l := range r.Layers {
		prefix := l.Scope.Kind + ":" + l.Scope.ID
		if applies(l.Policy.Deny, in.Action) {
			block("deny", prefix+":deny", "action denied by "+prefix)
		}
		for _, pattern := range l.Policy.ForbiddenPaths {
			for _, file := range in.Paths {
				if forbidden(pattern, file) {
					block("deny", prefix+":path:"+pattern, "forbidden path: "+file)
				}
			}
		}
		for _, req := range l.Policy.Required {
			if applies(req.Actions, in.Action) {
				key := requirementKey(req.ID, req.Identity)
				if n, ok := required[key]; !ok || req.Approvals > n {
					required[key] = req.Approvals
				}
			}
		}
	}
	base := map[Action][]string{Repair: {"execution_authority", "budget_capacity"}, Publish: {"execution_authority", "validation", "branch_ownership", "exact_head_guard"}, Merge: {"execution_authority", "validation", "native_rules", "native_reviews", "native_checks", "exact_head_guard", "target_enforcement", "merge_authority"}, Deploy: {"execution_authority", "native_approvals", "artifact_provenance", "workflow_authority"}, Recover: {"execution_authority", "native_approvals", "artifact_provenance", "recovery_authority", "known_good_artifact"}}
	if in.Stage == "deployment_admission" {
		base[Deploy] = []string{"execution_authority", "native_enforcement", "artifact_provenance", "workflow_authority"}
		base[Recover] = []string{"execution_authority", "native_enforcement", "artifact_provenance", "recovery_authority", "known_good_artifact"}
	}
	for _, id := range base[in.Action] {
		key := requirementKey(id, "")
		if _, ok := required[key]; !ok {
			required[key] = 0
		}
	}
	if in.Action == Publish || in.Action == Merge {
		if in.Current.Head == "" || in.Current.Target == "" || in.Current.Tested == "" {
			block("unknown", "candidate", "head, target and tested revision required")
		}
	}
	if in.Action == Merge || in.Action == Deploy || in.Action == Recover {
		if in.Current.ProviderRules == "" || in.Current.CapabilityVersion == "" {
			block("unknown", "provider", "current provider rules and capability version required")
		}
	}
	if in.Action == Deploy || in.Action == Recover {
		if in.Current.SourceSHA == "" || in.Current.Artifact == "" {
			block("unknown", "artifact", "source and artifact binding required")
		}
	}
	evidence := map[string]Evidence{}
	for _, e := range in.Evidence {
		key := requirementKey(e.ID, e.Identity)
		if _, ok := evidence[key]; ok {
			block("unknown", "evidence.duplicate", "duplicate evidence: "+e.ID)
		}
		evidence[key] = e
	}
	age := int64(300)
	if r.Policy.MaxEvidenceAgeSeconds != nil {
		age = *r.Policy.MaxEvidenceAgeSeconds
	}
	ids := []string{}
	for id := range required {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, key := range ids {
		parts := strings.SplitN(key, "\x00", 2)
		id := parts[0]
		if parts[1] != "" {
			id += "@" + parts[1]
		}
		e, ok := evidence[key]
		if !ok || e.Reference == "" || e.ObservedAt.IsZero() || e.ObservedAt.After(in.Now) || in.Now.Sub(e.ObservedAt).Seconds() > float64(age) || e.Binding != in.Current {
			block("unknown", id, "missing or stale evidence: "+id)
			continue
		}
		out.EvidenceReferences = append(out.EvidenceReferences, e.Reference)
		switch e.State {
		case "satisfied":
			if e.Approvals < required[key] {
				block("deny", id, "insufficient approvals: "+id)
			} else {
				out.Rules = append(out.Rules, id)
			}
		case "failed":
			block("deny", id, "requirement failed: "+id)
		default:
			block("unknown", id, "requirement not proven: "+id)
		}
	}
	return out
}

func ForbiddenPath(pattern, file string) bool { return forbidden(pattern, file) }
