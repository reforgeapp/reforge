package recipes

import (
	"slices"
	"strings"

	"reforge/internal/sandbox/guest"
)

const (
	ProofTests      = "tests"
	ProofStructural = "structural"
)

type Archetype struct {
	Name         string
	Toolchain    string
	MinProof     string
	ReviewOnly   bool
	Validator    string
	Categories   []string
	Ecosystems   []string
	AllowedPaths []string
}

var Toolchains = []string{"go", "javascript", "python"}

var archetypes = []Archetype{
	{Name: "go", Toolchain: "go", MinProof: ProofTests, Ecosystems: []string{"go", "gomod", "go_modules"}},
	{Name: "javascript", Toolchain: "javascript", MinProof: ProofTests, Ecosystems: []string{"npm", "yarn", "pnpm", "javascript"}},
	{Name: "python", Toolchain: "python", MinProof: ProofTests, Ecosystems: []string{"pip", "pypi", "python", "poetry"}},
	{Name: "config", MinProof: ProofStructural, Validator: "validate-bot-config", Categories: []string{"dependency_bots", "renovate_onboarding"}, AllowedPaths: guest.BotConfigPaths},
	{Name: "bootstrap", MinProof: ProofStructural, Validator: "validate-bootstrap", ReviewOnly: true, Categories: []string{"missing_validation"}},
}

func Lookup(name string) (Archetype, bool) {
	for _, a := range archetypes {
		if a.Name == name {
			return a, true
		}
	}
	return Archetype{}, false
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
