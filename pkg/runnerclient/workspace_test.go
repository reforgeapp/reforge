package runnerclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

func TestBootstrapPrefersMaintenanceImage(t *testing.T) {
	goImage := t.TempDir()
	maintenance := t.TempDir()
	for _, binary := range []string{"usr/local/go/bin/go", "usr/local/bin/node", "usr/local/bin/python3"} {
		for _, root := range []string{maintenance, goImage} {
			if root == goImage && binary != "usr/local/go/bin/go" {
				continue
			}
			name := filepath.Join(root, binary)
			if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, nil, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := sandbox.RuntimeConfig{Images: map[string]string{"go": goImage, "maintenance": maintenance}}
	p := repair.Plan{Image: "go", Recipe: recipes.Recipe{Name: "bootstrap"}}
	image := planImage(cfg, p)
	if image != "maintenance" {
		t.Fatalf("bootstrap image = %q, want maintenance", image)
	}
	runtime := &recordingSandboxRuntime{}
	request := sandbox.WorkspaceRequest{CommitSHA: "commit", Image: p.Image}
	prepare := commandPreparer{
		cfg:     cfg,
		runtime: runtime,
		image:   image,
		fetch: func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
			return sandbox.Snapshot{CommitSHA: "commit"}, nil
		},
	}
	_, err := prepare.prepare(context.Background(), request, nil, sandbox.Command{Args: []string{"/opt/reforge/tool", "validate-bootstrap"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.requests) != 1 || runtime.requests[0].Image != "maintenance" {
		t.Fatalf("prepared image = %+v, want maintenance", runtime.requests)
	}
}

func TestHasNpmLockfileFindsNearestWorkspaceAncestor(t *testing.T) {
	snapshot := sandbox.Snapshot{Files: []guest.File{
		{Path: "package.json"},
		{Path: "package-lock.json"},
		{Path: "apps/web/package.json"},
		{Path: "apps/web/package-lock.json"},
	}}
	if dir, ok := npmLockfileDirectory(snapshot, nil, "/workspace/apps/web/src"); !ok || dir != "apps/web" {
		t.Fatal("expected command directory ancestor lockfile")
	}
	if _, ok := npmLockfileDirectory(snapshot, []sandbox.Patch{{Path: "apps/web/package-lock.json", Delete: true}, {Path: "package-lock.json", Delete: true}}, "apps/web"); ok {
		t.Fatal("deleted lockfile must disable install")
	}
	if dir, ok := npmLockfileDirectory(snapshot, nil, "apps/other"); !ok || dir != "" {
		t.Fatal("expected root workspace lockfile")
	}
}

func TestApplySnapshotPatchesRefreshesManifestFiles(t *testing.T) {
	snapshot := sandbox.Snapshot{Files: []guest.File{
		{Path: "go.mod", Content: []byte("old"), Executable: true},
		{Path: "go.sum", Content: []byte("remove")},
	}}
	updated := applySnapshotPatches(snapshot, []sandbox.Patch{
		{Path: "go.mod", Content: []byte("new")},
		{Path: "go.sum", Delete: true},
		{Path: "nested/go.mod", Content: []byte("nested")},
	})
	got := map[string]guest.File{}
	for _, file := range updated.Files {
		got[file.Path] = file
	}
	if string(got["go.mod"].Content) != "new" || got["go.mod"].Executable {
		t.Fatal("patch must replace original file metadata and content")
	}
	if _, ok := got["go.sum"]; ok {
		t.Fatal("deleted file remains in refreshed snapshot")
	}
	if string(got["nested/go.mod"].Content) != "nested" {
		t.Fatal("new manifest missing from refreshed snapshot")
	}
	if string(snapshot.Files[0].Content) != "old" || !snapshot.Files[0].Executable || len(snapshot.Files) != 2 {
		t.Fatal("patch overlay mutated original snapshot")
	}
	digest, err := sandbox.SnapshotDigest(updated.Files)
	if err != nil || updated.ManifestSHA256 != digest {
		t.Fatal("patched snapshot digest is stale")
	}
}
