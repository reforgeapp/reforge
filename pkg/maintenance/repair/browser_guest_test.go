package repair

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

func TestBrowserEvidenceWithRealGVisor(t *testing.T) {
	configPath := os.Getenv("REFORGE_TEST_BROWSER_RUNTIME_CONFIG")
	if configPath == "" {
		t.Skip("requires approved local browser runtime and offline Playwright dependencies")
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config sandbox.RuntimeConfig
	if err = json.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Images) != 1 || config.DependencyRoot == "" {
		t.Fatal("requires one approved browser image and offline dependencies")
	}
	var image string
	for id := range config.Images {
		image = id
	}
	files := map[string][]byte{
		"package.json":          []byte(`{"scripts":{"test:e2e":"playwright test"}}`),
		"playwright.config.cjs": []byte(`module.exports = {testDir: '.', testMatch: 'checkout.spec.cjs', use: {viewport: {width: 390, height: 844}, screenshot: 'on', launchOptions: {args: ['--no-sandbox', '--disable-dev-shm-usage']}}};`),
		"checkout.js":           []byte(`document.querySelector('button').onclick = () => { document.querySelector('output').textContent = ['item'].length > 1 ? 'Order confirmed' : 'Nothing happened'; };`),
		"checkout.spec.cjs": []byte(`const {test, expect} = require('@playwright/test');
const fs = require('node:fs');
test('Checkout submits a single item at mobile width', async ({page}) => {
  await page.setContent('<button>Checkout</button><output></output>');
  await page.addScriptTag({content: fs.readFileSync('checkout.js', 'utf8')});
  await page.getByRole('button', {name: 'Checkout'}).click();
  await expect(page.locator('output')).toHaveText('Order confirmed', {timeout: 1000});
});`),
	}
	sha := strings.Repeat("a", 40)
	var sourceFiles []guest.File
	for name, data := range files {
		sourceFiles = append(sourceFiles, guest.File{Path: name, Content: data})
	}
	sort.Slice(sourceFiles, func(i, j int) bool { return sourceFiles[i].Path < sourceFiles[j].Path })
	manifest, err := sandbox.SnapshotDigest(sourceFiles)
	if err != nil {
		t.Fatal(err)
	}
	source := sandbox.Snapshot{CommitSHA: sha, ManifestSHA256: manifest, Complete: true, Files: sourceFiles}
	config.Fetch = func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) { return source, nil }
	runtime, err := sandbox.NewRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	plan, err := freeze("javascript", image, sha, sha, files, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Recipe.Commands) != 1 || plan.Recipe.Commands[0].ReportFormat != "playwright" {
		t.Fatal("existing browser script was not discovered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stored := map[string][]byte{}
	engine := Engine{Runtime: runtime, JobID: "browser-evidence", AttemptID: "qualification", Trust: "development-fixture", Artifact: func(_ context.Context, name string, data []byte) (string, string, error) {
		stored[name] = bytes.Clone(data)
		return name, hashBytes(data), nil
	}}
	engine.PrepareWorkspace = func(ctx context.Context, request sandbox.WorkspaceRequest, patches []sandbox.Patch, _ sandbox.Command) (sandbox.Workspace, error) {
		request.Dependencies = filepath.Join(config.DependencyRoot, "playwright")
		w, err := runtime.PreparePinnedWorkspace(ctx, request)
		if err != nil {
			return w, err
		}
		setup, err := runtime.ExecuteBoundedCommand(ctx, w, sandbox.Command{Args: []string{"cp", "-R", "/opt/deps/node_modules", "/workspace/node_modules"}, Directory: ".", Timeout: 30 * time.Second, MaxOutputBytes: 65536, NetworkProfile: "none"})
		if err == nil && setup.ExitCode == 0 && len(patches) > 0 {
			err = runtime.ApplyPatch(ctx, w, patches)
		}
		if err != nil || setup.ExitCode != 0 {
			_ = runtime.Destroy(context.WithoutCancel(ctx), w)
			if err == nil {
				err = &sandbox.CommandSetupError{Result: setup}
			}
		}
		return w, err
	}
	report := Report{}
	before, err := engine.validate(ctx, plan, sha, nil, "baseline", &report)
	if err != nil || !Reproduced(before) {
		t.Fatalf("mobile checkout failure unproved: checks=%+v err=%v", before, err)
	}
	patches := []sandbox.Patch{{Path: "checkout.js", Content: bytes.Replace(files["checkout.js"], []byte("length > 1"), []byte("length > 0"), 1)}}
	if err = CheckPatch(plan, files, patches); err != nil {
		t.Fatal(err)
	}
	after, err := engine.validate(ctx, plan, sha, patches, "candidate", &report)
	if err != nil || !Verified(plan, before, after) {
		t.Fatalf("mobile checkout fix unproved: checks=%+v err=%v", after, err)
	}
	for _, stage := range []string{"baseline", "candidate"} {
		png, trace := false, false
		for _, item := range report.BehaviourArtifacts {
			if item.Stage != stage {
				continue
			}
			if item.CheckSHA256 == "" || hashBytes(stored[item.ID]) != item.SHA256 {
				t.Fatal("browser evidence lost check attribution")
			}
			png = png || item.MediaType == "image/png"
			trace = trace || item.MediaType == "application/zip"
		}
		if !png || !trace {
			t.Fatalf("%s screenshot or trace missing: %+v", stage, report.BehaviourArtifacts)
		}
	}
	t.Logf("mobile checkout reproduced and fixed; %d attributed browser artifacts", len(report.BehaviourArtifacts))
}
