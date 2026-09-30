package recipes

import (
	"path"
	"slices"
	"strings"

	"reforge/internal/sandbox/guest"
)

const (
	ProofTests      = "tests"
	ProofStructural = "structural"
)

type Budget struct {
	Files, Lines, Bytes, GeneratedBytes, Turns int
}

var Ceiling = Budget{Files: 200, Lines: 50000, Bytes: 2 << 20, GeneratedBytes: 8 << 20, Turns: 300}

func (b Budget) Clamp() Budget {
	return Budget{min(b.Files, Ceiling.Files), min(b.Lines, Ceiling.Lines), min(b.Bytes, Ceiling.Bytes), min(b.GeneratedBytes, Ceiling.GeneratedBytes), min(b.Turns, Ceiling.Turns)}
}

var codeBudget = Budget{Files: 40, Lines: 20000, Bytes: 1 << 20, GeneratedBytes: 8 << 20, Turns: 200}

type Archetype struct {
	Name         string
	Toolchain    string
	MinProof     string
	ReviewOnly   bool
	ReadOnly     bool
	Validator    string
	Categories   []string
	Ecosystems   []string
	AllowedPaths []string
	Budget       Budget
}

var Toolchains = []string{"go", "javascript", "python"}

var archetypes = []Archetype{
	{Name: "go", Toolchain: "go", MinProof: ProofTests, Budget: codeBudget, Ecosystems: []string{"go", "gomod", "go_modules"}},
	{Name: "javascript", Toolchain: "javascript", MinProof: ProofTests, Budget: codeBudget, Ecosystems: []string{"npm", "yarn", "pnpm", "javascript"}},
	{Name: "python", Toolchain: "python", MinProof: ProofTests, Budget: codeBudget, Ecosystems: []string{"pip", "pypi", "python", "poetry"}},
	{Name: "config", MinProof: ProofStructural, Validator: "validate-bot-config", Budget: Budget{Files: 4, Lines: 400, Bytes: 64 << 10, Turns: 40}, Categories: []string{"dependency_bots", "renovate_onboarding"}, AllowedPaths: guest.BotConfigPaths},
	{Name: "bootstrap", MinProof: ProofStructural, Validator: "validate-bootstrap", ReviewOnly: true, Budget: Budget{Files: 30, Lines: 4000, Bytes: 256 << 10, GeneratedBytes: 2 << 20, Turns: 120}, Categories: []string{"missing_validation", "ci_gap", "test_gap"}},
	{Name: "docs", MinProof: ProofStructural, Validator: "validate-docs", Budget: Budget{Files: 20, Lines: 3000, Bytes: 256 << 10, Turns: 80}, Categories: []string{"docs_gap", "docs_drift"}, AllowedPaths: []string{"**/*.md", "docs/**"}},
	{Name: "review", MinProof: ProofStructural, Validator: "validate-bootstrap", ReadOnly: true, Budget: Budget{Files: 1, Lines: 1, Bytes: 1, Turns: 60}, Categories: []string{"repository_review"}},
}

func Lookup(name string) (Archetype, bool) {
	for _, a := range archetypes {
		if a.Name == name {
			return a, true
		}
	}
	return Archetype{}, false
}

func GeneratedPath(file string) bool {
	lower := strings.ToLower(file)
	name := path.Base(lower)
	for _, part := range strings.Split(path.Dir(lower), "/") {
		if part == "vendor" || part == "node_modules" || part == "generated" || part == "dist" {
			return true
		}
	}
	return slices.Contains([]string{"go.sum", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "poetry.lock", "uv.lock", "cargo.lock", "gemfile.lock", "composer.lock"}, name) || strings.HasSuffix(name, ".pb.go") || strings.Contains(name, "_generated.") || strings.Contains(name, ".gen.")
}

func Archetypes() []Archetype { return slices.Clone(archetypes) }

func ForFinding(category, ecosystem string) []string {
	for _, a := range archetypes {
		if slices.Contains(a.Categories, category) {
			return []string{a.Name}
		}
	}
	out := []string{}
	for _, a := range archetypes {
		if len(a.Categories) == 0 && slices.Contains(a.Ecosystems, strings.ToLower(ecosystem)) {
			out = append(out, a.Name)
		}
	}
	for _, a := range archetypes {
		if len(a.Categories) == 0 && !slices.Contains(out, a.Name) {
			out = append(out, a.Name)
		}
	}
	return out
}

func Images(toolchains map[string]string) map[string]string {
	out := map[string]string{}
	for _, a := range archetypes {
		if a.Toolchain != "" {
			if digest := toolchains[a.Toolchain]; digest != "" {
				out[a.Name] = digest
			}
			continue
		}
		for _, t := range Toolchains {
			if digest := toolchains[t]; digest != "" {
				out[a.Name] = digest
				break
			}
		}
	}
	return out
}
