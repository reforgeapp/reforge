package gitops

import (
	"bytes"
	"strings"
	"testing"
)

func TestPatchManifestNestedYAMLPreservesUnrelatedBytes(t *testing.T) {
	source := []byte("apiVersion: v1\r\nmetadata:\r\n  name: app\r\nspec:\r\n  containers:\r\n    - name: web\r\n      image: old/image:1\r\n  replicas: 2\r\n")
	got, err := PatchManifest(source, "/spec/containers/0/image", "old/image:1", "new/image:2")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("apiVersion: v1\r\nmetadata:\r\n  name: app\r\nspec:\r\n  containers:\r\n    - name: web\r\n      image: new/image:2\r\n  replicas: 2\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	if value, err := ReadManifestField(got, "/spec/containers/0/image"); err != nil || value != "new/image:2" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestPatchManifestJSONAndRFC6901(t *testing.T) {
	source := []byte(`{"metadata":{"labels":{"app/name":"old"}},"items":[{"image":"one"},{"image":"two"}]}`)
	got, err := PatchManifest(source, "/metadata/labels/app~1name", "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"metadata":{"labels":{"app/name":"new"}},"items":[{"image":"one"},{"image":"two"}]}` {
		t.Fatalf("unexpected JSON: %s", got)
	}
	got, err = PatchManifest(got, "/items/1/image", "two", "three")
	if err != nil || !strings.Contains(string(got), `"image":"three"`) {
		t.Fatalf("got=%s err=%v", got, err)
	}
}

func TestPatchManifestDigestAndUnicodePrefix(t *testing.T) {
	source := []byte("title: café\nimage: registry/app@sha256:old\nother: registry/app@sha256:old\n")
	got, err := PatchManifest(source, "/image", "registry/app@sha256:old", "registry/app@sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "title: café\n") || !strings.Contains(string(got), "other: registry/app@sha256:old\n") {
		t.Fatalf("unrelated values changed: %s", got)
	}
	value, err := ReadManifestField(got, "/image")
	if err != nil || value != "registry/app@sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestPatchManifestRejectsAmbiguityAndUnsafeTargets(t *testing.T) {
	cases := []struct{ name, source, pointer, expected, desired string }{
		{"mismatch", "image: old\n", "/image", "other", "new"},
		{"duplicate", "image: old\nimage: other\n", "/image", "old", "new"},
		{"alias", "base: &x old\nimage: *x\n", "/image", "old", "new"},
		{"multiline", "image: |\n  old\n", "/image", "old", "new"},
		{"bad pointer", "image: old\n", "/image~2", "old", "new"},
		{"nonstring", "replicas: 2\n", "/replicas", "2", "3"},
		{"multiline desired", "image: old\n", "/image", "old", "new\nvalue"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PatchManifest([]byte(tc.source), tc.pointer, tc.expected, tc.desired); err == nil {
				t.Fatal("unsafe or ambiguous patch accepted")
			}
		})
	}
}

func TestPatchManifestRejectsMultipleDocumentsAndOversize(t *testing.T) {
	if _, err := PatchManifest([]byte("image: old\n---\nimage: other\n"), "/image", "old", "new"); err == nil {
		t.Fatal("multiple documents accepted")
	}
	if _, err := PatchManifest([]byte(strings.Repeat("x", maxManifestSize+1)), "/image", "x", "y"); err == nil {
		t.Fatal("oversize manifest accepted")
	}
}

func TestManifestScalarArrayAndRawMultilineBoundary(t *testing.T) {
	got, err := PatchManifest([]byte(`{"items":["old","old"],"caption":"café"}`), "/items/1", "old", "new")
	if err != nil || string(got) != `{"items":["old","new"],"caption":"café"}` {
		t.Fatalf("array scalar: %s %v", got, err)
	}
	for _, body := range []string{"image: \"old\\nvalue\"\n", "image: \"old\n value\"\n", "image: !!str old\n"} {
		if _, err := ReadManifestField([]byte(body), "/image"); err == nil {
			t.Fatalf("unsafe scalar accepted: %q", body)
		}
	}
}
