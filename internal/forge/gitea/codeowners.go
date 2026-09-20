package gitea

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type CodeOwnerRule struct {
	Pattern  string   `json:"pattern"`
	Owners   []string `json:"owners"`
	Negative bool     `json:"negative"`
}
type CodeOwners struct {
	Path        string                 `json:"path"`
	CommitSHA   string                 `json:"commit_sha"`
	Rules       []CodeOwnerRule        `json:"rules"`
	Enforcement domain.CapabilityState `json:"enforcement"`
}

func ownerFields(line string) []string {
	var fields []string
	var word strings.Builder
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) && (runes[i+1] == '#' || runes[i+1] == '\\' || runes[i+1] == ' ') {
			i++
			word.WriteRune(runes[i])
			continue
		}
		if r == '#' {
			break
		}
		if unicode.IsSpace(r) {
			if word.Len() > 0 {
				fields = append(fields, word.String())
				word.Reset()
			}
			continue
		}
		word.WriteRune(r)
	}
	if word.Len() > 0 {
		fields = append(fields, word.String())
	}
	return fields
}
func ParseCodeOwners(content []byte) ([]CodeOwnerRule, error) {
	if len(content) > 256<<10 {
		return nil, failure("invalid", "CODEOWNERS exceeds parsing limit")
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) > 2000 {
		return nil, failure("invalid", "CODEOWNERS has too many lines")
	}
	var out []CodeOwnerRule
	for _, line := range lines {
		fields := ownerFields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 2 || len(fields) > 101 || len(fields[0]) > 4096 {
			return nil, failure("invalid", "Invalid Gitea CODEOWNERS rule")
		}
		rule := CodeOwnerRule{Pattern: fields[0], Owners: append([]string(nil), fields[1:]...)}
		if strings.HasPrefix(rule.Pattern, "!") {
			rule.Negative = true
			rule.Pattern = strings.TrimPrefix(rule.Pattern, "!")
		}
		if rule.Pattern == "" {
			return nil, failure("invalid", "Empty CODEOWNERS expression")
		}
		if _, e := regexp.Compile(rule.Pattern); e != nil {
			return nil, failure("invalid", "Invalid Gitea CODEOWNERS regular expression")
		}
		for _, owner := range rule.Owners {
			if !strings.HasPrefix(owner, "@") || len(owner) < 2 || len(owner) > 256 || strings.ContainsAny(owner, " \t\r\n\\") || strings.Count(owner, "/") > 1 {
				return nil, failure("invalid", "Invalid CODEOWNERS owner reference")
			}
		}
		out = append(out, rule)
	}
	return out, nil
}
func (p *Provider) ReadCodeOwners(ctx context.Context, r forge.RepoRef, commit string) (CodeOwners, error) {
	out := CodeOwners{CommitSHA: commit, Enforcement: domain.Unknown}
	for _, name := range []string{"CODEOWNERS", "docs/CODEOWNERS", ".gitea/CODEOWNERS"} {
		file, e := p.ReadFileAtRef(ctx, r, name, commit)
		if e != nil {
			if pe, ok := e.(*domain.ProviderError); ok && pe.Kind == "not_found" {
				continue
			}
			return out, e
		}
		out.Path = name
		out.Rules, e = ParseCodeOwners(file.Content)
		return out, e
	}
	return out, failure("not_found", "No Gitea CODEOWNERS file at immutable commit")
}
