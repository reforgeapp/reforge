package artifact

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestBrowserEvidenceRejectsTraversalSecretsAndNonTraceArchives(t *testing.T) {
	var screenshot bytes.Buffer
	if err := png.Encode(&screenshot, image.NewRGBA(image.Rect(0, 0, 30, 20))); err != nil {
		t.Fatal(err)
	}
	if !validContent("image/png", screenshot.Bytes()) || validContent("image/jpeg", screenshot.Bytes()) || validContent("image/png", []byte("fake image")) {
		t.Fatal("image validation does not match decoded format")
	}
	archive := func(files map[string][]byte) []byte {
		var data bytes.Buffer
		writer := zip.NewWriter(&data)
		for name, body := range files {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			entry.Write(body)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return data.Bytes()
	}
	if !validContent("application/zip", archive(map[string][]byte{"0.trace": []byte(`{"type":"action","title":"Checkout"}`), "resources/page.png": screenshot.Bytes()})) {
		t.Fatal("bounded browser trace rejected")
	}
	frame, media := TraceScreenshot(archive(map[string][]byte{"0.trace": []byte("{\"type\":\"screencast-frame\",\"sha1\":\"page.png\"}\n"), "resources/page.png": screenshot.Bytes()}))
	if media != "image/png" || !bytes.Equal(frame, screenshot.Bytes()) {
		t.Fatal("executed trace frame unavailable")
	}
	for name, files := range map[string]map[string][]byte{
		"escape":               {"../0.trace": []byte("trace")},
		"credential":           {"0.trace": []byte(`{"Authorization":"Bearer secret-value-123456789"}`)},
		"cookie header":        {"0.trace": []byte(`{"headers":[{"name":"cookie","value":"session=private-value"}]}`)},
		"compressed oversized": {"0.trace": bytes.Repeat([]byte("x"), int(MaxSize)+1)},
		"arbitrary zip":        {"app.js": []byte("code")},
		"nested zip":           {"0.trace": []byte("trace"), "nested.zip": []byte("zip")},
	} {
		data := archive(files)
		if validContent("application/zip", data) {
			t.Fatalf("unsafe evidence accepted: %s", name)
		}
		if frame, _ := TraceScreenshot(data); len(frame) > 0 {
			t.Fatalf("unsafe trace extracted: %s", name)
		}
	}
}
