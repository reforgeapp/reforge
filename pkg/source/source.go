package source

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

const MaxEntries = 10000
const MaxFileBytes = 64 << 20
const MaxTotalBytes = 64 << 20

type Reader struct {
	Manifest func(context.Context, forge.RepoRef, string) (forge.SourceManifest, error)
	File     func(context.Context, forge.RepoRef, string, string) (forge.File, error)
	Files    func(context.Context, forge.RepoRef, []string, string) ([]forge.File, error)
	Cache    *BlobCache
	Scope    string
}

func ValidSHA(value, format string) bool {
	size := 40
	if format == "sha256" {
		size = 64
	} else if format != "sha1" {
		return false
	}
	if len(value) != size || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func objectSHA(format, kind string, content []byte) string {
	var h hash.Hash
	if format == "sha256" {
		h = sha256.New()
	} else {
		h = sha1.New()
	}
	fmt.Fprintf(h, "%s %d\x00", kind, len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func VerifyBlob(format, expected string, content []byte) bool {
	return ValidSHA(expected, format) && objectSHA(format, "blob", content) == expected
}

func ValidEntry(e forge.SourceEntry, format string) bool {
	if !guest.ValidPath(e.Path) || !utf8.ValidString(e.Path) || !ValidSHA(e.SHA, format) || e.Size < 0 {
		return false
	}
	for _, part := range strings.Split(e.Path, "/") {
		if strings.EqualFold(part, ".git") || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return false
		}
	}
	return e.Type == "tree" && e.Mode == "040000" || e.Type == "blob" && (e.Mode == "100644" || e.Mode == "100755")
}

func ValidateManifest(ctx context.Context, m forge.SourceManifest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !m.Complete || m.Repository.NativeID == "" || m.Repository.FullName == "" || !ValidSHA(m.CommitSHA, m.ObjectFormat) || len(m.Entries) > MaxEntries {
		return "", errors.New("incomplete or invalid source manifest")
	}
	switch m.Proof {
	case "commit_tree_hash":
		if !ValidSHA(m.TreeSHA, m.ObjectFormat) {
			return "", errors.New("native commit tree hash required")
		}
	case "immutable_ref_api":
		if m.TreeSHA != "" {
			return "", errors.New("immutable ref proof must not claim native root tree hash")
		}
	default:
		return "", errors.New("unqualified source proof")
	}
	entries := make(map[string]forge.SourceEntry, len(m.Entries))
	folded := make(map[string]bool, len(m.Entries))
	children := make(map[string][]forge.SourceEntry)
	for _, e := range m.Entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !ValidEntry(e, m.ObjectFormat) {
			return "", errors.New("invalid source entry; symlinks and submodules are disabled")
		}
		key := strings.ToLower(e.Path)
		if folded[key] {
			return "", errors.New("duplicate or case-colliding source path")
		}
		folded[key] = true
		entries[e.Path] = e
		parent := path.Dir(e.Path)
		if parent == "." {
			parent = ""
		}
		children[parent] = append(children[parent], e)
	}
	for name := range entries {
		parent := path.Dir(name)
		if parent != "." && entries[parent].Type != "tree" {
			return "", errors.New("missing directory or file-directory collision")
		}
	}
	var tree func(string) (string, error)
	tree = func(dir string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		rows := children[dir]
		sort.Slice(rows, func(i, j int) bool {
			a, b := path.Base(rows[i].Path), path.Base(rows[j].Path)
			if rows[i].Type == "tree" {
				a += "/"
			}
			if rows[j].Type == "tree" {
				b += "/"
			}
			return a < b
		})
		var content bytes.Buffer
		for _, e := range rows {
			mode := e.Mode
			if e.Type == "tree" {
				got, err := tree(e.Path)
				if err != nil {
					return "", err
				}
				if got != e.SHA {
					return "", errors.New("source subtree hash mismatch")
				}
				mode = "40000"
			}
			raw, _ := hex.DecodeString(e.SHA)
			fmt.Fprintf(&content, "%s %s\x00", mode, path.Base(e.Path))
			content.Write(raw)
		}
		return objectSHA(m.ObjectFormat, "tree", content.Bytes()), nil
	}
	root, err := tree("")
	if err != nil {
		return "", err
	}
	if m.Proof == "commit_tree_hash" && root != m.TreeSHA {
		return "", errors.New("source root tree hash mismatch")
	}
	return root, nil
}

func Fetch(ctx context.Context, reader Reader, repo forge.RepoRef, commit string) (sandbox.Snapshot, error) {
	var zero sandbox.Snapshot
	if reader.Manifest == nil || reader.File == nil && reader.Files == nil || repo.NativeID == "" || repo.FullName == "" || !(ValidSHA(commit, "sha1") || ValidSHA(commit, "sha256")) {
		return zero, errors.New("authorized source readers and repository identity required")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	manifest, err := reader.Manifest(ctx, repo, commit)
	if err != nil {
		return zero, err
	}
	if manifest.Repository != repo || manifest.CommitSHA != commit {
		return zero, errors.New("source repository or commit identity mismatch")
	}
	if _, err = ValidateManifest(ctx, manifest); err != nil {
		return zero, err
	}
	entries := append([]forge.SourceEntry(nil), manifest.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	blobs := entries[:0]
	for _, e := range entries {
		if e.Type == "blob" {
			blobs = append(blobs, e)
		}
	}
	contents := map[string][]byte{}
	missing := []forge.SourceEntry{}
	for _, e := range blobs {
		if data, ok := reader.Cache.get(reader.Scope, manifest.ObjectFormat, e.SHA); ok {
			contents[e.Path] = data
		} else {
			missing = append(missing, e)
		}
	}
	accept := func(e forge.SourceEntry, f forge.File) error {
		if f.Path != e.Path || f.SHA != "" && f.SHA != e.SHA || len(f.Content) > MaxFileBytes || e.Size > 0 && int64(len(f.Content)) != e.Size {
			return errors.New("invalid or oversized source file")
		}
		content := bytes.Clone(f.Content)
		if objectSHA(manifest.ObjectFormat, "blob", content) != e.SHA {
			return errors.New("source blob hash mismatch")
		}
		contents[e.Path] = content
		reader.Cache.put(reader.Scope, manifest.ObjectFormat, e.SHA, content)
		return nil
	}
	for done := 0; done < len(missing); {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if reader.Files == nil {
			f, err := reader.File(ctx, repo, missing[done].Path, commit)
			if err != nil {
				return zero, err
			}
			if err = accept(missing[done], f); err != nil {
				return zero, err
			}
			done++
			continue
		}
		paths := []string{}
		for _, e := range missing[done:min(len(missing), done+50)] {
			paths = append(paths, e.Path)
		}
		batch, err := reader.Files(ctx, repo, paths, commit)
		if err != nil {
			return zero, err
		}
		if len(batch) == 0 || len(batch) > len(paths) {
			return zero, errors.New("invalid source file batch")
		}
		for _, f := range batch {
			if err = accept(missing[done], f); err != nil {
				return zero, err
			}
			done++
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	files := make([]guest.File, 0, len(blobs))
	total := 0
	for _, e := range blobs {
		content := contents[e.Path]
		if len(content) > MaxTotalBytes-total {
			return zero, errors.New("invalid or oversized source file")
		}
		total += len(content)
		files = append(files, guest.File{Path: e.Path, Content: bytes.Clone(content), Executable: e.Mode == "100755"})
	}
	digest, err := sandbox.SnapshotDigest(files)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return sandbox.Snapshot{CommitSHA: commit, Complete: true, ManifestSHA256: digest, Files: files}, nil
}
