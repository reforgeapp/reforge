package artifact

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path"
	"regexp"
	"strings"
)

var traceCredentials = regexp.MustCompile(`(?i)"(?:cookie|set-cookie|authorization|proxy-authorization)"`)

func EvidenceMedia(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".zip":
		return "application/zip"
	}
	return "text/plain"
}

func validImage(media string, data []byte) bool {
	if sensitive.Match(data) {
		return false
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || "image/"+format != media || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 8<<20 {
		return false
	}
	_, _, err = image.Decode(bytes.NewReader(data))
	return err == nil
}

func validTrace(data []byte) bool {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > 256 {
		return false
	}
	total := uint64(0)
	trace := false
	seen := map[string]bool{}
	for _, file := range reader.File {
		name := file.Name
		if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00") || !file.Mode().IsRegular() || seen[name] || file.UncompressedSize64 > uint64(MaxSize) {
			return false
		}
		seen[name] = true
		total += file.UncompressedSize64
		if total > 16<<20 {
			return false
		}
		stream, err := file.Open()
		if err != nil {
			return false
		}
		body, err := io.ReadAll(io.LimitReader(stream, MaxSize+1))
		stream.Close()
		if err != nil || len(body) > int(MaxSize) {
			return false
		}
		media := EvidenceMedia(name)
		if media == "application/zip" || media != "text/plain" && !validImage(media, body) || media == "text/plain" && (!validContent(media, body) || traceCredentials.Match(body)) {
			return false
		}
		trace = trace || strings.HasSuffix(name, ".trace")
	}
	return trace
}

func TraceScreenshot(data []byte) ([]byte, string) {
	if len(data) > int(MaxSize) || !validTrace(data) {
		return nil, ""
	}
	reader, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	frame := ""
	for _, file := range reader.File {
		if !strings.HasSuffix(file.Name, ".trace") {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return nil, ""
		}
		body, err := io.ReadAll(io.LimitReader(stream, MaxSize+1))
		stream.Close()
		if err != nil {
			return nil, ""
		}
		for _, line := range bytes.Split(body, []byte("\n")) {
			var event struct {
				Type string `json:"type"`
				SHA1 string `json:"sha1"`
			}
			if json.Unmarshal(line, &event) == nil && event.Type == "screencast-frame" && path.Base(event.SHA1) == event.SHA1 {
				frame = "resources/" + event.SHA1
			}
		}
	}
	for _, file := range reader.File {
		if file.Name != frame {
			continue
		}
		media := EvidenceMedia(file.Name)
		if media != "image/png" && media != "image/jpeg" {
			return nil, ""
		}
		stream, err := file.Open()
		if err != nil {
			return nil, ""
		}
		body, err := io.ReadAll(io.LimitReader(stream, MaxSize+1))
		stream.Close()
		if err != nil || !validImage(media, body) {
			return nil, ""
		}
		return body, media
	}
	return nil, ""
}
