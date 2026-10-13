package repair

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	"github.com/reforgeapp/reforge/pkg/artifact"
	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

type BehaviourArtifact struct {
	ID          string `json:"id"`
	Stage       string `json:"stage"`
	Name        string `json:"name"`
	MediaType   string `json:"media_type"`
	SHA256      string `json:"sha256"`
	CheckSHA256 string `json:"check_sha256"`
}

func (e Engine) browserArtifacts(ctx context.Context, w sandbox.Workspace, command recipes.Command, result sandbox.CommandResult, label string, report *Report) error {
	if e.Artifact == nil || command.ReportFormat != "playwright" || label == "native" {
		return nil
	}
	parsed := guest.ParseTestReport("playwright", result.Output)
	if !parsed.Complete {
		return nil
	}
	prefix := path.Join(command.Directory, ".reforge/browser", command.ID) + "/"
	seen := map[string]bool{}
	count := 0
	for _, item := range report.BehaviourArtifacts {
		if item.Stage == behaviourStage(label) {
			count++
		}
	}
	bytes := 0
	upload := func(name, media string, data []byte) {
		if count >= 8 || bytes+len(data) > 8<<20 {
			return
		}
		id, digest, err := e.Artifact(ctx, name, data)
		if err != nil {
			e.log(ctx, "tool", "Browser artifact unavailable: "+name)
			return
		}
		bytes += len(data)
		count++
		report.Artifacts = append(report.Artifacts, id)
		report.BehaviourArtifacts = append(report.BehaviourArtifacts, BehaviourArtifact{ID: id, Stage: behaviourStage(label), Name: name, MediaType: media, SHA256: digest})
	}
	for index, attachment := range parsed.Attachments {
		if count >= 8 {
			break
		}
		file := attachment.Path
		media := attachment.ContentType
		if media != "image/png" && media != "image/jpeg" && media != "application/zip" {
			continue
		}
		collected := sandbox.Artifact{}
		inline := file == "" && attachment.Body != ""
		if inline {
			if len(attachment.Body) > base64.StdEncoding.EncodedLen(int(artifact.MaxSize)) {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(attachment.Body)
			if err != nil {
				continue
			}
			collected.Data = data
			ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "application/zip": ".zip"}[media]
			file = prefix + fmt.Sprintf("attachment-%d", index+1) + ext
		}
		if strings.HasPrefix(file, "/workspace/") {
			file = strings.TrimPrefix(file, "/workspace/")
		} else if strings.HasPrefix(file, "/") {
			continue
		}
		file = path.Clean(file)
		if !guest.ValidPath(file) || !strings.HasPrefix(file, prefix) || seen[file] {
			continue
		}
		if artifact.EvidenceMedia(file) != media {
			continue
		}
		if !inline {
			var err error
			collected, err = e.Runtime.CollectArtifact(ctx, w, file)
			if err != nil {
				e.log(ctx, "tool", "Browser artifact unavailable: "+path.Base(file))
				continue
			}
		}
		if len(collected.Data) > int(artifact.MaxSize) || bytes+len(collected.Data) > 8<<20 {
			break
		}
		if media == "image/png" && (len(collected.Data) < 8 || string(collected.Data[:8]) != "\x89PNG\r\n\x1a\n") || media == "image/jpeg" && (len(collected.Data) < 3 || string(collected.Data[:3]) != "\xff\xd8\xff") || media == "application/zip" && (len(collected.Data) < 4 || string(collected.Data[:4]) != "PK\x03\x04") {
			continue
		}
		name := label + "-" + command.ID + "-" + path.Base(file)
		seen[file] = true
		if media == "application/zip" {
			if frame, frameMedia := artifact.TraceScreenshot(collected.Data); len(frame) > 0 {
				ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg"}[frameMedia]
				upload(label+"-"+command.ID+"-frame"+ext, frameMedia, frame)
			}
		}
		upload(name, media, collected.Data)
	}
	return nil
}

func behaviourStage(label string) string {
	switch {
	case strings.HasPrefix(label, "reported-defect-before"):
		return "reproduction"
	case label == "baseline", label == "target-baseline":
		return "baseline"
	case strings.Contains(label, "candidate"):
		return "candidate"
	case strings.HasPrefix(label, "target"):
		return "target"
	default:
		return label
	}
}
