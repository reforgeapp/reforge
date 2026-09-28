package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"reforge/internal/auth"
	"reforge/internal/maintenance/recipes"
)

func fingerprint(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func validate(in Input) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 160 || len(in.Selection) > 2000 || len(in.Members) < 1 || len(in.Members) > 1000 || in.CanarySize < 1 || in.CanarySize > 100 || in.CanarySize > len(in.Members) || in.BatchSize < 1 || in.BatchSize > 100 || in.Concurrency < 1 || in.Concurrency > 100 || in.ObservationSeconds < 0 || in.ObservationSeconds > 86400 || in.FailureLimit < 0 || in.FailureLimit > 1000 || in.FailurePercent < 0 || in.FailurePercent > 100 || len(in.CanaryIDs) > 100 || len(in.Windows) > 14 {
		return auth.ErrInvalid
	}
	if in.Kind == "repair" {
		if in.Success != "published" && in.Success != "merged" {
			return auth.ErrInvalid
		}
	} else if (in.Kind != "pipeline" && in.Kind != "gitops") || in.Success != "healthy" {
		return auth.ErrInvalid
	}
	seen := map[string]bool{}
	for _, m := range in.Members {
		if !auth.ValidID(m.RepositoryID) || seen[m.RepositoryID] {
			return auth.ErrInvalid
		}
		seen[m.RepositoryID] = true
		switch in.Kind {
		case "repair":
			if m.Repair == nil || m.Pipeline != nil || m.GitOps != nil || m.Environment != "" {
				return auth.ErrInvalid
			}
			r := m.Repair
			if !auth.ValidID(r.FindingID) || r.FindingVersion < 1 || !auth.ValidID(r.ModelConnectionID) || !auth.ValidID(r.RunnerPoolID) || !slices.Contains(recipes.Toolchains, r.Recipe) || r.ModelRoute == "" || len(r.ModelRoute) > 100 || r.IdempotencyKey != "" || r.PlanDigest != "" {
				return auth.ErrInvalid
			}
		case "pipeline":
			if m.Pipeline == nil || m.Repair != nil || m.GitOps != nil || m.Environment == "" || len(m.Environment) > 100 || m.Pipeline.RecoveryOf != "" || m.Pipeline.RestoreDeploymentID != "" {
				return auth.ErrInvalid
			}
		case "gitops":
			if m.GitOps == nil || m.Repair != nil || m.Pipeline != nil || m.Environment == "" || len(m.Environment) > 100 || m.GitOps.RecoveryOf != "" || m.GitOps.RestorePromotionID != "" {
				return auth.ErrInvalid
			}
		}
	}
	canaries := map[string]bool{}
	for _, id := range in.CanaryIDs {
		if !seen[id] || canaries[id] {
			return auth.ErrInvalid
		}
		canaries[id] = true
	}
	if len(in.CanaryIDs) > 0 && len(in.CanaryIDs) != in.CanarySize {
		return auth.ErrInvalid
	}
	for _, w := range in.Windows {
		if len(w.Weekdays) < 1 || len(w.Weekdays) > 7 || w.StartMinute < 0 || w.EndMinute > 1440 || w.EndMinute <= w.StartMinute {
			return auth.ErrInvalid
		}
		seenDays := map[int]bool{}
		for _, day := range w.Weekdays {
			if day < 0 || day > 6 || seenDays[day] {
				return auth.ErrInvalid
			}
			seenDays[day] = true
		}
	}
	return nil
}

func withinWindow(in Input, now time.Time) bool {
	if in.NotBefore != nil && now.Before(*in.NotBefore) {
		return false
	}
	if len(in.Windows) == 0 {
		return true
	}
	now = now.UTC()
	minute := now.Hour()*60 + now.Minute()
	for _, w := range in.Windows {
		if slices.Contains(w.Weekdays, int(now.Weekday())) && minute >= w.StartMinute && minute < w.EndMinute {
			return true
		}
	}
	return false
}

func assignCanaries(in Input, members []Member) []string {
	groups := map[string]bool{}
	chosen := map[string]bool{}
	eligible := 0
	for _, m := range members {
		if m.State != "excluded" {
			eligible++
			groups[m.Group] = false
		}
	}
	if eligible < in.CanarySize {
		return []string{"Canary size exceeds eligible members; resolve exclusions or create a new selection"}
	}
	if len(groups) > in.CanarySize {
		return []string{fmt.Sprintf("At least %d canaries are required to represent recipe, forge version and validation groups", len(groups))}
	}
	if len(in.CanaryIDs) > 0 {
		for _, id := range in.CanaryIDs {
			chosen[id] = true
		}
		for _, m := range members {
			if chosen[m.RepositoryID] {
				if m.State == "excluded" {
					return []string{"An explicitly selected canary is excluded"}
				}
				groups[m.Group] = true
			}
		}
		for _, covered := range groups {
			if !covered {
				return []string{"Explicit canaries must cover every eligible recipe, forge version and validation group"}
			}
		}
	} else {
		for _, m := range members {
			if m.State != "excluded" && !groups[m.Group] {
				chosen[m.RepositoryID] = true
				groups[m.Group] = true
			}
		}
		for _, m := range members {
			if len(chosen) >= in.CanarySize {
				break
			}
			if m.State != "excluded" {
				chosen[m.RepositoryID] = true
			}
		}
	}
	stage, used := 2, 0
	for i := range members {
		m := &members[i]
		m.Canary = chosen[m.RepositoryID]
		if m.State == "excluded" {
			continue
		}
		if m.Canary {
			m.Stage = 1
			continue
		}
		m.Stage = stage
		used++
		if used == in.BatchSize {
			stage++
			used = 0
		}
	}
	return []string{}
}

func summarize(members []Member) Counts {
	c := Counts{Total: len(members)}
	for _, m := range members {
		switch m.State {
		case "excluded":
			c.Excluded++
		case "pending":
			c.Pending++
		case "queued", "running", "observing":
			c.Running++
		case "succeeded":
			c.Succeeded++
		case "failed", "cancelled":
			c.Failed++
		case "blocked", "unknown":
			c.Unknown++
		}
	}
	return c
}
