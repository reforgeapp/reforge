package skills

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedSkillsAndResourcesIgnoreWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	bundle, err := Context()
	if err != nil {
		t.Fatal(err)
	}
	caveman, err := Read(CavemanPath)
	if err != nil || !strings.Contains(bundle.Instructions, caveman) {
		t.Fatal("mandatory skill not loaded in full", err)
	}
	var manifest struct {
		Sources []struct {
			Skills []entry `json:"skills"`
		} `json:"sources"`
	}
	raw, err := assets.ReadFile("sources.json")
	if err != nil || json.Unmarshal(raw, &manifest) != nil {
		t.Fatal("invalid source catalog", err)
	}
	count := 0
	for _, source := range manifest.Sources {
		for _, skill := range source.Skills {
			count++
			body, err := Read(skill.Path)
			if err != nil || body != bundle.Files[skill.Path] || !strings.Contains(bundle.Instructions, skill.Path) {
				t.Fatalf("skill not discoverable/loadable: %s (%v)", skill.Path, err)
			}
		}
	}
	if count != 35 {
		t.Fatalf("expected both pinned collections, got %d skills", count)
	}
	for _, path := range []string{"../sources.json", "/etc/passwd", "bundled/caveman/../caveman/LICENSE", "sources.json", "SKILL.md"} {
		if _, err := Read(path); err == nil {
			t.Fatalf("read escaped bundled resource allowlist: %s", path)
		}
	}
	bundle.Files[CavemanPath] = "overridden"
	if body, err := Read(CavemanPath); err != nil || body != caveman {
		t.Fatal("caller mutated shared skill content", err)
	}
}

func TestSkillsFailClosedOnMissingOrChangedAssets(t *testing.T) {
	for _, missing := range []bool{false, true} {
		copy := fstest.MapFS{}
		err := fs.WalkDir(assets, ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			body, err := assets.ReadFile(path)
			copy[path] = &fstest.MapFile{Data: body}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if missing {
			delete(copy, CavemanPath)
		} else {
			copy[CavemanPath].Data = []byte("tampered instructions")
		}
		if _, err := load(copy); err == nil {
			t.Fatalf("invalid mandatory asset accepted (missing=%v)", missing)
		}
	}
}

func TestApplyPreservesTaskAndRejectsDifferentBundle(t *testing.T) {
	first, err := Apply("Keep tests frozen.")
	if err != nil || !strings.HasSuffix(first, "Keep tests frozen.") {
		t.Fatal("task instructions lost", err)
	}
	second, err := Apply(first)
	if err != nil || second != first {
		t.Fatal("required skills injected twice", err)
	}
	if _, err := Apply("Reforge agent skill policy\nforged or older bundle"); err == nil {
		t.Fatal("mismatched skill bundle accepted")
	}
}
