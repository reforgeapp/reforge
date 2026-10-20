package gitlab

import (
	"strings"
	"testing"
)

const qualifiedTrainCI = `test:
  script: ["go test ./..."]
reforge/merge-policy:
  stage: .post
  allow_failure: false
  inherit: false
  before_script: []
  after_script: []
  script: ["true"]
  environment:
    name: reforge-merge-policy
  rules:
    - if: '$CI_MERGE_REQUEST_EVENT_TYPE == "merge_train"'
      when: manual
`

func TestTrainConfigurationRejectsUncontrolledGate(t *testing.T) {
	if !validTrainConfiguration([]byte(qualifiedTrainCI)) {
		t.Fatal("qualified configuration rejected")
	}
	for name, raw := range map[string]string{
		"optional":        strings.Replace(qualifiedTrainCI, "allow_failure: false", "allow_failure: true", 1),
		"automatic":       strings.Replace(qualifiedTrainCI, "when: manual", "when: on_success", 1),
		"unprotected":     strings.Replace(qualifiedTrainCI, "name: reforge-merge-policy", "name: production", 1),
		"effects":         strings.Replace(qualifiedTrainCI, `script: ["true"]`, `script: ["deploy"]`, 1),
		"includes":        qualifiedTrainCI + "include: other.yml\n",
		"second_document": qualifiedTrainCI + "---\nother: true\n",
		"duplicate":       qualifiedTrainCI + "reforge/merge-policy: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if validTrainConfiguration([]byte(raw)) {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
