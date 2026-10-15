package guest

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
		default:
			var object map[string]any
			if err := json.Unmarshal(body, &object); err != nil || object == nil {
				problems = append(problems, name+": cannot validate this syntax; use a JSON object in renovate.json")
			} else if schema, err := renovateConfigSchema(); err != nil {
				problems = append(problems, name+": Renovate schema validation unavailable")
			} else if err := schema.Validate(object); err != nil {
				problems = append(problems, fmt.Sprintf("%s: Renovate %s configuration invalid: %.2000s", name, renovateSchemaVersion, err))
			}
		}
	}
	if !found {
		problems = append(problems, "no Dependabot or Renovate configuration")
	}
	return problems
}

var renovateConfigSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	compressed, err := base64.StdEncoding.DecodeString(renovateSchemaGzip)
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	body, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	properties, ok := document["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("Renovate schema properties unavailable")
	}
	properties["$schema"] = map[string]any{"type": "string"}
	document["additionalProperties"] = false
	compiler := jsonschema.NewCompiler()
	const uri = "https://docs.renovatebot.com/renovate-schema.json"
	if err := compiler.AddResource(uri, document); err != nil {
		return nil, err
	}
	return compiler.Compile(uri)
})

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
