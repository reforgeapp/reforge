package release

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"

	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func CollectValidation(target, format string, build, test sandbox.CommandResult) (*deployment.BuildValidation, error) {
	if build.ExitCode != 0 || build.Duration <= 0 || test.Duration <= 0 || build.TimedOut || build.Truncated || !slices.Contains([]string{"json", "text", "tap", "jest", "playwright", "bootstrap"}, format) {
		return nil, errors.New("successful bounded build and supported test report required")
	}
	check := repair.Interpret(recipes.Command{ID: "native-ci", ReportFormat: format}, test)
	digest := func(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
	validation := &deployment.BuildValidation{TargetSHA: target, Builds: 1, Complete: check.Complete && check.ExitCode == 0, Cases: check.Cases, Outputs: []string{digest(build.Output), digest(test.Output)}}
	if !Meaningful(deployment.Provenance{Validation: validation}) {
		return nil, errors.New("build did not produce complete passing named behavioural checks")
	}
	return validation, nil
}
