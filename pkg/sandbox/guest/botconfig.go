package guest

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"

	"github.com/goccy/go-yaml"
)

var BotConfigPaths = []string{
	".github/dependabot.yml", ".github/dependabot.yaml",
	"renovate.json", "renovate.json5", ".renovaterc", ".renovaterc.json", ".renovaterc.json5",
	".github/renovate.json", ".github/renovate.json5", ".gitlab/renovate.json", ".gitlab/renovate.json5",
}

func ValidateBotConfig(root fs.FS) []string {
	problems := []string{}
	found := false
	for _, name := range BotConfigPaths {
		body, err := fs.ReadFile(root, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		found = true
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			continue
		}
		switch {
		case strings.Contains(name, "dependabot"):
			problems = append(problems, dependabotProblems(name, body)...)
		case strings.HasSuffix(name, ".json5"):
		default:
			var object map[string]any
			if err := json.Unmarshal(body, &object); err != nil || object == nil {
				problems = append(problems, name+": not a JSON object")
			}
		}
	}
	if !found {
		problems = append(problems, "no Dependabot or Renovate configuration")
	}
	return problems
}

func dependabotProblems(name string, body []byte) []string {
	var config struct {
		Version int `yaml:"version"`
		Updates []struct {
			Ecosystem   string         `yaml:"package-ecosystem"`
			Directory   string         `yaml:"directory"`
			Directories []string       `yaml:"directories"`
			Schedule    map[string]any `yaml:"schedule"`
			Groups      map[string]any `yaml:"groups"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(body, &config); err != nil {
		return []string{name + ": " + err.Error()}
	}
	problems := []string{}
	if config.Version != 2 {
		problems = append(problems, name+": version must be 2")
	}
	if len(config.Updates) == 0 {
		problems = append(problems, name+": updates is empty")
	}
	grouped := false
	for _, u := range config.Updates {
		if u.Ecosystem == "" || u.Directory == "" && len(u.Directories) == 0 || u.Schedule["interval"] == nil {
			problems = append(problems, name+": each update needs package-ecosystem, directory and schedule.interval")
		}
		grouped = grouped || len(u.Groups) > 0
	}
	if !grouped {
		problems = append(problems, name+": no update groups")
	}
	return problems
}
