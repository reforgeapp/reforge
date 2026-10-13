package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/release"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestCaptureCollectsRealBehaviourAndRejectsFailedIncompleteOutput(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("requires Node test runner")
	}
	file := filepath.Join(t.TempDir(), "checkout.test.cjs")
	if err = os.WriteFile(file, []byte("require('node:test')('one item checkout',()=>require('node:assert/strict').equal(1+1,2))"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = run([]string{"capture", "--", node, "--test", "--test-reporter=tap", "--test-isolation=none", file}, &output); err != nil {
		t.Fatal(err)
	}
	var test sandbox.CommandResult
	if err = json.Unmarshal(output.Bytes(), &test); err != nil {
		t.Fatal(err)
	}
	validation, err := release.CollectValidation("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "tap", sandbox.CommandResult{Duration: time.Millisecond, Output: []byte("built checkout")}, test)
	if err != nil || len(validation.Cases) != 1 || !validation.Complete {
		t.Fatalf("actual check lost: %+v %v", validation, err)
	}
	if err = os.WriteFile(file, []byte("require('node:test')('one item checkout',()=>require('node:assert/strict').equal(1,2))"), 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if run([]string{"capture", "--", node, "--test", "--test-reporter=tap", "--test-isolation=none", file}, &output) == nil {
		t.Fatal("failed CI command reported success")
	}
	if err = json.Unmarshal(output.Bytes(), &test); err != nil {
		t.Fatal(err)
	}
	if _, err = release.CollectValidation("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "tap", sandbox.CommandResult{Duration: time.Millisecond}, test); err == nil {
		t.Fatal("failed behaviour signed as successful")
	}
	for _, args := range [][]string{{"capture", "--max-output-bytes", "8", "--", node, "-e", "console.log('more than eight bytes')"}, {"capture", "--timeout", "20ms", "--", node, "-e", "setInterval(()=>{},1000)"}} {
		output.Reset()
		if run(args, &output) == nil {
			t.Fatal("incomplete CI command reported success")
		}
		if err = json.Unmarshal(output.Bytes(), &test); err != nil {
			t.Fatal(err)
		}
		if !test.Truncated && !test.TimedOut {
			t.Fatal("incomplete command qualification lost")
		}
	}
}
