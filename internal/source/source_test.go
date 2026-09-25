package source

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"reforge/internal/forge"
)

func fixture(format string) (forge.SourceManifest, map[string][]byte) {
	data := map[string][]byte{"a/file": []byte("hello\n"), "a.c": []byte("code\n"), "a0": []byte("exec\n")}
	entries := []forge.SourceEntry{{Path: "a", Mode: "040000", Type: "tree"}, {Path: "a/file", Mode: "100644", Type: "blob"}, {Path: "a.c", Mode: "100644", Type: "blob"}, {Path: "a0", Mode: "100755", Type: "blob"}}
	for i := 1; i < len(entries); i++ {
		entries[i].SHA = objectSHA(format, "blob", data[entries[i].Path])
	}
	raw := make([]byte, 0)
	b, _ := hex.DecodeString(entries[1].SHA)
	raw = append(raw, []byte("100644 file\x00")...)
	raw = append(raw, b...)
	entries[0].SHA = objectSHA(format, "tree", raw)
	raw = nil
	for _, i := range []int{2, 0, 3} {
		mode := entries[i].Mode
		if i == 0 {
			mode = "40000"
		}
		raw = append(raw, []byte(mode+" "+entries[i].Path+"\x00")...)
		b, _ = hex.DecodeString(entries[i].SHA)
		raw = append(raw, b...)
	}
	commit := strings.Repeat("a", 40)
	if format == "sha256" {
		commit = strings.Repeat("a", 64)
	}
	return forge.SourceManifest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, CommitSHA: commit, TreeSHA: objectSHA(format, "tree", raw), ObjectFormat: format, Proof: "commit_tree_hash", Complete: true, Entries: entries}, data
}

func readerFor(m forge.SourceManifest, data map[string][]byte) Reader {
	return Reader{Manifest: func(context.Context, forge.RepoRef, string) (forge.SourceManifest, error) { return m, nil }, File: func(_ context.Context, r forge.RepoRef, p, c string) (forge.File, error) {
		if r != m.Repository || c != m.CommitSHA {
			return forge.File{}, errors.New("unbound read")
		}
		return forge.File{Path: p, Content: data[p]}, nil
	}}
}

func TestPinnedSnapshotChecksObjectsOrderingModesAndProof(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			m, data := fixture(format)
			snapshot, err := Fetch(context.Background(), readerFor(m, data), m.Repository, m.CommitSHA)
			if err != nil || !snapshot.Complete || len(snapshot.Files) != 3 || !snapshot.Files[2].Executable || snapshot.Files[0].Path != "a.c" {
				t.Fatalf("snapshot=%+v error=%v", snapshot, err)
			}
			data["a.c"][0] = 'X'
			if snapshot.Files[0].Content[0] != 'c' {
				t.Fatal("snapshot aliases source bytes")
			}
			if _, err = Fetch(context.Background(), readerFor(m, data), m.Repository, m.CommitSHA); err == nil {
				t.Fatal("corrupt blob accepted")
			}
			m, data = fixture(format)
			m.Proof = "immutable_ref_api"
			m.TreeSHA = ""
			if _, err = Fetch(context.Background(), readerFor(m, data), m.Repository, m.CommitSHA); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManifestRejectsIncompleteHostileAndUnqualifiedSourcesBeforeContent(t *testing.T) {
	mutations := map[string]func(*forge.SourceManifest){
		"incomplete":        func(m *forge.SourceManifest) { m.Complete = false },
		"root":              func(m *forge.SourceManifest) { m.TreeSHA = strings.Repeat("0", 40) },
		"subtree":           func(m *forge.SourceManifest) { m.Entries[0].SHA = strings.Repeat("0", 40) },
		"missing file":      func(m *forge.SourceManifest) { m.Entries = m.Entries[:len(m.Entries)-1] },
		"missing directory": func(m *forge.SourceManifest) { m.Entries = m.Entries[1:] },
		"duplicate":         func(m *forge.SourceManifest) { m.Entries = append(m.Entries, m.Entries[1]) },
		"case collision":    func(m *forge.SourceManifest) { e := m.Entries[1]; e.Path = "A/FILE"; m.Entries = append(m.Entries, e) },
		"traversal":         func(m *forge.SourceManifest) { m.Entries[1].Path = "../outside" },
		"nested git":        func(m *forge.SourceManifest) { m.Entries[1].Path = "a/.GIT/config" },
		"symlink":           func(m *forge.SourceManifest) { m.Entries[1].Mode = "120000" },
		"submodule":         func(m *forge.SourceManifest) { m.Entries[1].Mode = "160000"; m.Entries[1].Type = "commit" },
		"collision":         func(m *forge.SourceManifest) { m.Entries[0].Mode = "100644"; m.Entries[0].Type = "blob" },
		"format":            func(m *forge.SourceManifest) { m.ObjectFormat = "guess" },
		"proof":             func(m *forge.SourceManifest) { m.Proof = "archive" },
		"false root claim":  func(m *forge.SourceManifest) { m.Proof = "immutable_ref_api" },
		"repository":        func(m *forge.SourceManifest) { m.Repository.NativeID = "2" },
		"commit":            func(m *forge.SourceManifest) { m.CommitSHA = strings.Repeat("b", 40) },
		"entries limit":     func(m *forge.SourceManifest) { m.Entries = make([]forge.SourceEntry, MaxEntries+1) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			m, data := fixture("sha1")
			r, c := m.Repository, m.CommitSHA
			mutate(&m)
			reader := readerFor(m, data)
			reader.File = func(context.Context, forge.RepoRef, string, string) (forge.File, error) {
				t.Fatal("unverified manifest fetched contents")
				return forge.File{}, nil
			}
			if _, err := Fetch(context.Background(), reader, r, c); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
}

func TestSourceCancellationContentLimitsAndIdentity(t *testing.T) {
	for _, name := range []string{"cancel-before", "cancel-read", "file-limit", "wrong-path", "wrong-hash", "mutable-ref"} {
		t.Run(name, func(t *testing.T) {
			m, data := fixture("sha1")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := readerFor(m, data)
			base := r.File
			r.File = func(ctx context.Context, repo forge.RepoRef, p, c string) (forge.File, error) {
				f, e := base(ctx, repo, p, c)
				switch name {
				case "cancel-read":
					cancel()
				case "file-limit":
					f.Content = make([]byte, MaxFileBytes+1)
				case "wrong-path":
					f.Path = "other"
				case "wrong-hash":
					f.SHA = strings.Repeat("b", 40)
				}
				return f, e
			}
			if name == "cancel-before" {
				cancel()
			}
			commit := m.CommitSHA
			if name == "mutable-ref" {
				commit = "main"
				r.Manifest = func(context.Context, forge.RepoRef, string) (forge.SourceManifest, error) {
					t.Fatal("mutable ref reached reader")
					return m, nil
				}
			}
			if _, err := Fetch(ctx, r, m.Repository, commit); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestSnapshotTotalByteLimit(t *testing.T) {
	m, _ := fixture("sha1")
	m.Entries = nil
	m.TreeSHA = ""
	m.Proof = "immutable_ref_api"
	data := make([]byte, MaxFileBytes)
	sha := objectSHA("sha1", "blob", data)
	for i := 0; i < 17; i++ {
		m.Entries = append(m.Entries, forge.SourceEntry{Path: fmt.Sprintf("file-%02d", i), SHA: sha, Mode: "100644", Type: "blob"})
	}
	reader := readerFor(m, nil)
	calls := 0
	reader.File = func(_ context.Context, _ forge.RepoRef, p, _ string) (forge.File, error) {
		calls++
		return forge.File{Path: p, Content: data}, nil
	}
	snapshot, err := Fetch(context.Background(), reader, m.Repository, m.CommitSHA)
	if err == nil || calls != 17 || snapshot.Complete || len(snapshot.Files) != 0 {
		t.Fatalf("total byte limit failed calls=%d error=%v", calls, err)
	}
}

func TestBatchedFilesContinueFromPartialBatches(t *testing.T) {
	m, data := fixture("sha1")
	reader := readerFor(m, data)
	reader.File = nil
	calls := 0
	reader.Files = func(_ context.Context, _ forge.RepoRef, paths []string, _ string) ([]forge.File, error) {
		calls++
		return []forge.File{{Path: paths[0], Content: data[paths[0]]}}, nil
	}
	snapshot, err := Fetch(context.Background(), reader, m.Repository, m.CommitSHA)
	if err != nil || len(snapshot.Files) != 3 || calls != 3 || snapshot.Files[0].Path != "a.c" {
		t.Fatalf("snapshot=%+v calls=%d error=%v", snapshot, calls, err)
	}
	reader.Files = func(_ context.Context, _ forge.RepoRef, paths []string, _ string) ([]forge.File, error) {
		return []forge.File{{Path: paths[len(paths)-1], Content: data[paths[len(paths)-1]]}}, nil
	}
	if _, err = Fetch(context.Background(), reader, m.Repository, m.CommitSHA); err == nil {
		t.Fatal("out-of-order batch accepted")
	}
}

func TestBlobCacheAvoidsRepeatReads(t *testing.T) {
	m, data := fixture("sha1")
	reader := readerFor(m, data)
	reads := 0
	file := reader.File
	reader.File = func(ctx context.Context, r forge.RepoRef, p, c string) (forge.File, error) {
		reads++
		return file(ctx, r, p, c)
	}
	reader.Cache, reader.Scope = NewBlobCache(1<<20), "org"
	for range 2 {
		if _, err := Fetch(context.Background(), reader, m.Repository, m.CommitSHA); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 3 {
		t.Fatalf("reads=%d", reads)
	}
	reader.Scope = "other"
	if _, err := Fetch(context.Background(), reader, m.Repository, m.CommitSHA); err != nil || reads != 6 {
		t.Fatalf("scope not isolated: reads=%d %v", reads, err)
	}
}
