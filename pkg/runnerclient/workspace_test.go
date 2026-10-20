package runnerclient

import (
	"testing"

	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

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
