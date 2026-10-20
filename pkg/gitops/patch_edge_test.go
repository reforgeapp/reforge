package gitops

import (
	"bytes"
	"strings"
	"testing"
)

func TestPatchManifestPreservesCommentsQuotesAndUnicode(t *testing.T) {
	source := []byte("title: \"café\"\r\nimage: 'old#tag' # retained\r\n")
	got, err := PatchManifest(source, "/image", "old#tag", "new#tag")
	if err != nil {
		t.Fatal(err)
	}
	want := "title: \"café\"\r\nimage: 'new#tag' # retained\r\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if value, err := ReadManifestField(got, "/title"); err != nil || value != "café" {
		t.Fatalf("title=%q err=%v", value, err)
	}
}

func TestPatchManifestArrayPointerRejectsNonCanonicalIndex(t *testing.T) {
	source := []byte("items:\n  - name: \"café\"\n  - name: old\n")
	got, err := PatchManifest(source, "/items/0/name", "café", "新")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := ReadManifestField(got, "/items/0/name"); err != nil || value != "新" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	for _, pointer := range []string{"/items/01/name", "/items/2/name", "/items/-1/name"} {
		if _, err := PatchManifest(source, pointer, "old", "new"); err == nil {
			t.Fatalf("accepted invalid array pointer %q", pointer)
		}
	}
}

func TestPatchManifestRejectsInvalidUTF8AndNestedAliases(t *testing.T) {
	invalid := append([]byte("image: old\n"), 0xff)
	if _, err := PatchManifest(invalid, "/image", "old", "new"); err == nil {
		t.Fatal("accepted invalid UTF-8 manifest")
	}
	alias := []byte("root:\n  image: &base old\n  copy:\n    image: *base\n")
	if _, err := PatchManifest(alias, "/root/image", "old", "new"); err == nil {
		t.Fatal("accepted nested YAML alias")
	}
}

func TestPatchManifestDeepInputDoesNotPanic(t *testing.T) {
	const depth = 256
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString(strings.Repeat("  ", i))
		b.WriteString("a:\n")
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("image: old\n")
	pointer := "/" + strings.Repeat("a/", depth) + "image"
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("deep manifest panicked: %v", recovered)
		}
	}()
	got, err := PatchManifest([]byte(b.String()), pointer, "old", "new")
	if err == nil {
		value, readErr := ReadManifestField(got, pointer)
		if readErr != nil || value != "new" {
			t.Fatalf("deep value=%q err=%v", value, readErr)
		}
	}
}

func FuzzPatchManifestBounded(f *testing.F) {
	seeds := []struct {
		body, pointer, expected, desired string
	}{
		{"image: old\r\n# keep\r\n", "/image", "old", "new"},
		{"items:\n  - image: old\n", "/items/0/image", "old", "registry/app@sha256:" + strings.Repeat("a", 64)},
		{"{\"items\":[\"old\",\"keep\"]}", "/items/0", "old", "new"},
		{"title: \"café\"\nimage: 'old#tag'\n", "/image", "old#tag", "new#tag"},
	}
	for _, seed := range seeds {
		f.Add(seed.body, seed.pointer, seed.expected, seed.desired)
	}
	f.Fuzz(func(t *testing.T, body, pointer, expected, desired string) {
		if len(body) > maxManifestSize || len(pointer) > 4096 || len(expected) > 4096 || len(desired) > 4096 {
			t.Skip()
		}
		got, err := PatchManifest([]byte(body), pointer, expected, desired)
		if err != nil {
			return
		}
		value, err := ReadManifestField(got, pointer)
		if err != nil || value != desired {
			t.Fatalf("patched value=%q err=%v want=%q", value, err, desired)
		}
		again, err := PatchManifest([]byte(body), pointer, expected, desired)
		if err != nil || !bytes.Equal(got, again) {
			t.Fatalf("nondeterministic patch: first=%q second=%q err=%v", got, again, err)
		}
	})
}

func TestPatchManifestBoundsFlowAndBlockNesting(t *testing.T) {
	for _, body := range []string{strings.Repeat("[", 5000) + "old" + strings.Repeat("]", 5000), "root: " + strings.Repeat("{a: ", 5000) + "old" + strings.Repeat("}", 5000)} {
		if _, err := PatchManifest([]byte(body), "/root", "old", "new"); err == nil || !strings.Contains(err.Error(), "nesting limit") {
			t.Fatalf("deep flow not bounded: %v", err)
		}
	}
}
