package release

import (
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestCollectCIProofRejectsEmptySkippedFailedAndUnavailableTests(t *testing.T) {
	target := strings.Repeat("a", 40)
	build := sandbox.CommandResult{Duration: time.Millisecond, Output: []byte("compiled checkout")}
	good := sandbox.CommandResult{Duration: time.Millisecond, Output: []byte("ok 1 - checkout submits order at mobile width\n")}
	proof, err := CollectValidation(target, "tap", build, good)
	if err != nil || proof.Cases["1:checkout submits order at mobile width"] != "pass" || proof.Outputs[0] == proof.Outputs[1] {
		t.Fatalf("CI behaviour not collected: %+v %v", proof, err)
	}
	for name, result := range map[string]sandbox.CommandResult{
		"empty":              {Output: []byte("echo ok")},
		"file only":          {Output: []byte("ok 1 - empty.test.js\n")},
		"skip":               {Output: []byte("ok 1 - checkout # SKIP\n")},
		"failed":             {ExitCode: 1, Output: []byte("not ok 1 - checkout\n")},
		"truncated":          {Truncated: true, Output: good.Output},
		"missing dependency": {ExitCode: 127},
	} {
		result.Duration = time.Millisecond
		if _, err := CollectValidation(target, "tap", build, result); err == nil {
			t.Fatalf("%s became native proof", name)
		}
	}
	build.ExitCode = 1
	if _, err := CollectValidation(target, "tap", build, good); err == nil {
		t.Fatal("failed build certified")
	}
}
