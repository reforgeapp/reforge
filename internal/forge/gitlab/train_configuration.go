package gitlab

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"

	"github.com/goccy/go-yaml"
	"reforge/internal/forge"
)

const trainEnvironment = "reforge-merge-policy"

func validTrainConfiguration(raw []byte) bool {
	if len(raw) == 0 || len(raw) > 256*1024 {
		return false
	}
	var document map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&document) != nil || decoder.Decode(new(any)) != io.EOF {
		return false
	}
	if _, ok := document["include"]; ok {
		return false
	}
	gate, ok := document[forge.QueueExecutionCheckName].(map[string]any)
	if !ok {
		return false
	}
	expected := map[string]any{
		"stage": ".post", "allow_failure": false, "inherit": false,
		"before_script": []any{}, "after_script": []any{}, "script": []any{"true"},
		"environment": map[string]any{"name": trainEnvironment},
		"rules":       []any{map[string]any{"if": "$CI_MERGE_REQUEST_EVENT_TYPE == \"merge_train\"", "when": "manual"}},
	}
	for key, value := range expected {
		if !reflect.DeepEqual(gate[key], value) {
			return false
		}
	}
	for key := range gate {
		if _, ok := expected[key]; !ok && key != "image" && key != "tags" {
			return false
		}
	}
	return true
}

func (p *Provider) verifyTrainConfiguration(ctx context.Context, r forge.RepoRef, change forge.Change, candidate string) (string, error) {
	var baseline []byte
	for _, sha := range []string{change.HeadSHA, change.TargetSHA, candidate} {
		file, err := p.ReadFileAtRef(ctx, r, ".gitlab-ci.yml", sha)
		if err != nil {
			return "", err
		}
		if baseline == nil {
			baseline = file.Content
		} else if !bytes.Equal(baseline, file.Content) {
			return "", failure("conflict", "Train CI configuration changed across H/T/C")
		}
	}
	if !validTrainConfiguration(baseline) {
		return "", failure("unsupported", "Train gate requires the documented immutable standalone CI configuration")
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return "", err
	}
	var project struct {
		ID         int64           `json:"id"`
		ConfigPath json.RawMessage `json:"ci_config_path"`
	}
	if err = p.read(ctx, []string{"projects", r.NativeID}, &project); err != nil {
		return "", err
	}
	if stringID(project.ID) != r.NativeID || len(project.ConfigPath) == 0 || string(project.ConfigPath) != "null" && string(project.ConfigPath) != `""` && string(project.ConfigPath) != `".gitlab-ci.yml"` {
		return "", failure("unsupported", "Default project CI configuration is not proven")
	}
	rows, err := p.protectionReader().pages(ctx, []string{"projects", r.NativeID, "protected_environments"}, nil)
	if err != nil {
		return "", err
	}
	matched := false
	for _, raw := range rows {
		var environment struct {
			Name     string            `json:"name"`
			Required *int              `json:"required_approval_count"`
			Rules    []json.RawMessage `json:"approval_rules"`
			Access   []struct {
				ID    int64  `json:"id"`
				User  *int64 `json:"user_id"`
				Group *int64 `json:"group_id"`
				Level *int   `json:"access_level"`
			} `json:"deploy_access_levels"`
		}
		if json.Unmarshal(raw, &environment) != nil || environment.Name == "" {
			return "", failure("identity", "Protected environment identity is incomplete")
		}
		if !matches(environment.Name, trainEnvironment) {
			continue
		}
		if environment.Name != trainEnvironment || environment.Required == nil || *environment.Required != 0 || len(environment.Rules) != 0 || len(environment.Access) != 1 {
			return "", failure("unsupported", "Train gate requires an exact protected environment with one operational user and no separate approval gate")
		}
		access := environment.Access[0]
		if access.ID <= 0 || access.User == nil || stringID(*access.User) != actor || access.Group != nil || access.Level != nil && *access.Level != 0 {
			return "", failure("unsupported", "Train manual job can be released by another actor")
		}
		matched = true
	}
	if !matched {
		return "", failure("unsupported", "Protected train gate environment missing")
	}
	hash := sha256.Sum256(baseline)
	return hex.EncodeToString(hash[:]), nil
}
