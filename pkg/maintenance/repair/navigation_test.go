package repair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestReadSnapshotChunkCoversMaximumTextFile(t *testing.T) {
	body := []byte(strings.Repeat("x", navigationFileLimit))
	files := map[string][]byte{"src/main.go": body}
	var collected strings.Builder
	offset := 0
	for {
		raw, err := readSnapshotChunk(files, "src/main.go", offset, navigationOutputLimit)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > navigationOutputLimit {
			t.Fatalf("response size %d exceeds limit", len(raw))
		}
		var chunk navigationFileChunk
		if err = json.Unmarshal(raw, &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Offset != offset || chunk.NextOffset <= offset && !chunk.Done {
			t.Fatalf("invalid chunk progress: %+v", chunk)
		}
		collected.WriteString(chunk.Content)
		offset = chunk.NextOffset
		if chunk.Done {
			break
		}
	}
	if collected.String() != string(body) {
		t.Fatalf("read %d bytes, want %d", collected.Len(), len(body))
	}
}

func TestReadSnapshotChunkPreservesUnicodeAndBoundsEscapedOutput(t *testing.T) {
	files := map[string][]byte{"unicode.txt": []byte("aébc")}
	first, err := readSnapshotChunk(files, "unicode.txt", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	var chunk navigationFileChunk
	if err = json.Unmarshal(first, &chunk); err != nil || chunk.Content != "a" || chunk.NextOffset != 1 {
		t.Fatalf("first chunk %+v, err %v", chunk, err)
	}
	second, err := readSnapshotChunk(files, "unicode.txt", chunk.NextOffset, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(second, &chunk); err != nil || chunk.Content != "é" || chunk.NextOffset != 3 {
		t.Fatalf("second chunk %+v, err %v", chunk, err)
	}
	escaped := map[string][]byte{"quoted.txt": []byte(strings.Repeat("\"", navigationOutputLimit))}
	raw, err := readSnapshotChunk(escaped, "quoted.txt", 0, navigationOutputLimit)
	if err != nil || len(raw) > navigationOutputLimit {
		t.Fatalf("escaped chunk size %d, err %v", len(raw), err)
	}
	if err = json.Unmarshal(raw, &chunk); err != nil || chunk.NextOffset == 0 || !utf8.ValidString(chunk.Content) {
		t.Fatalf("escaped chunk failed to progress: %+v, err %v", chunk, err)
	}
}

func TestSnapshotPathAndLiteralSearchNavigationIsDeterministic(t *testing.T) {
	files := map[string][]byte{
		"src/b.go":  []byte("other\nneedle two\n"),
		"src/a.go":  []byte("needle one\nnone\nneedle three\n"),
		"README.md": []byte("needle ignored by glob\n"),
	}
	firstPaths, err := listSnapshotPaths(files, "src/*.go", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var paths navigationPathPage
	if err = json.Unmarshal(firstPaths, &paths); err != nil || len(paths.Paths) != 1 || paths.Paths[0] != "src/a.go" || paths.Done {
		t.Fatalf("first path page %+v, err %v", paths, err)
	}
	nextPaths, err := listSnapshotPaths(files, "src/*.go", paths.NextOffset, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(nextPaths, &paths); err != nil || len(paths.Paths) != 1 || paths.Paths[0] != "src/b.go" || !paths.Done {
		t.Fatalf("second path page %+v, err %v", paths, err)
	}
	firstMatches, err := searchSnapshotContent(files, "needle", "src/*.go", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var matches navigationSearchPage
	if err = json.Unmarshal(firstMatches, &matches); err != nil || len(matches.Matches) != 1 || matches.Matches[0].Path != "src/a.go" || matches.Matches[0].Line != 1 || matches.Done {
		t.Fatalf("first search page %+v, err %v", matches, err)
	}
	secondMatches, err := searchSnapshotContent(files, "needle", "src/*.go", matches.NextOffset, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(secondMatches, &matches); err != nil || len(matches.Matches) != 1 || matches.Matches[0].Path != "src/a.go" || matches.Matches[0].Line != 3 {
		t.Fatalf("second search page %+v, err %v", matches, err)
	}
}

func TestSnapshotNavigationDoesNotExposeSecretFilesOrMaterial(t *testing.T) {
	files := map[string][]byte{
		".env.local":    []byte("TOKEN=secret\n"),
		"src/config.go": []byte("private key material: -----BEGIN PRIVATE KEY-----\n"),
		"src/main.go":   []byte("safe source\n"),
	}
	if _, err := readSnapshotChunk(files, ".env.local", 0, 1024); err == nil {
		t.Fatal("secret file readable")
	}
	raw, err := searchSnapshotContent(files, "secret", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	var page navigationSearchPage
	if err = json.Unmarshal(raw, &page); err != nil || len(page.Matches) != 0 {
		t.Fatalf("secret search results %+v, err %v", page, err)
	}
}

func TestSnapshotNavigationRejectsInvalidRangesAndPatterns(t *testing.T) {
	files := map[string][]byte{"file.txt": []byte("étext")}
	if _, err := readSnapshotChunk(files, "file.txt", 1, navigationOutputLimit); err == nil {
		t.Fatal("non-rune byte offset accepted")
	}
	if _, err := listSnapshotPaths(files, "[", 0, 1); err == nil {
		t.Fatal("invalid glob accepted")
	}
	if _, err := searchSnapshotContent(files, "", "", 0, 1); err == nil {
		t.Fatal("empty search accepted")
	}
}

func TestOwnerNavigationReadsLargeFileThenStagesAndValidatesEdit(t *testing.T) {
	plan, baseFiles := testPlan(t)
	plan.Owner = true
	plan.MaxChangedLines = 1 << 20
	plan.Recipe.MaxFiles = 20
	plan.Recipe.MaxPatchBytes = 768 << 10
	plan.Recipe.MaxTurns = 2
	plan.Digest = planDigest(plan)
	targetFiles := make(map[string][]byte, len(baseFiles)+1)
	for name, body := range baseFiles {
		targetFiles[name] = body
	}
	targetFiles["src/large.go"] = []byte(strings.Repeat("x", 65530) + "\nneedle-old\n" + strings.Repeat("z", 2048))
	runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: targetFiles}, target: targetFiles, targetSHA: plan.TargetSHA}
	turn := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "navigation", AttemptID: "navigation", Trust: "fixture", MaxOutputTokens: 256, TurnTimeout: time.Second, Progress: func(context.Context, string) error { return nil }}
	engine.Turn = func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		turn++
		if turn == 1 {
			return model.TurnResult{ToolCalls: []model.ToolCall{
				{ID: "paths", Name: "list_files", Arguments: []byte(`{"glob":"src/*.go","limit":10}`)},
				{ID: "search", Name: "search_files", Arguments: []byte(`{"query":"needle-old","glob":"src/*.go","limit":10}`)},
				{ID: "large-read", Name: "read_file", Arguments: []byte(`{"path":"src/large.go","offset":65530,"limit":4096}`)},
				{ID: "edit", Name: "edit_file", Arguments: []byte(`{"path":"src/large.go","old":"needle-old","new":"needle-new"}`)},
				{ID: "staged-read", Name: "read_file", Arguments: []byte(`{"path":"src/large.go","offset":65530,"limit":4096}`)},
				{ID: "checks", Name: "run_checks", Arguments: []byte(`{}`)},
			}}, nil
		}
		for _, message := range in.Messages {
			switch message.ToolCallID {
			case "paths":
				if !strings.Contains(message.Text, "src/large.go") {
					t.Fatal("path navigation missed large file")
				}
			case "search":
				if !strings.Contains(message.Text, "needle-old") {
					t.Fatal("literal search missed marker")
				}
			case "large-read":
				if !strings.Contains(message.Text, "needle-old") {
					t.Fatal("offset read missed large-file marker")
				}
			case "edit":
				if !strings.Contains(message.Text, "staged") {
					t.Fatalf("edit was rejected: %s", message.Text)
				}
			case "staged-read":
				if !strings.Contains(message.Text, "needle-new") {
					t.Fatalf("read_file did not show staged content: %s", message.Text)
				}
			case "checks":
				if !strings.Contains(message.Text, `"complete":true`) {
					t.Fatalf("owner checks did not complete: %s", message.Text)
				}
			}
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Update large source file"}`)}}}, nil
	}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, baseFiles), snapshotForEngine(t, plan.TargetSHA, targetFiles))
	if err != nil || report.State != "validated" || turn != 2 || len(report.Patches) != 1 || !strings.Contains(string(report.Patches[0].Content), "needle-new") {
		t.Fatalf("report=%+v turns=%d error=%v", report, turn, err)
	}
}

func TestOwnerPromptBoundsLargeInventoryAndReadsOmittedCILogSections(t *testing.T) {
	plan, baseFiles := testPlan(t)
	plan.Owner = true
	plan.Recipe.MaxTurns = 2
	plan.Digest = planDigest(plan)
	targetFiles := make(map[string][]byte, len(baseFiles)+12000)
	for name, body := range baseFiles {
		targetFiles[name] = body
	}
	for i := 0; i < 12000; i++ {
		name := fmt.Sprintf("src/%05d_%s/file.go", i, strings.Repeat("x", 28))
		targetFiles[name] = []byte("package fixture\n")
	}
	log := strings.Repeat("a", 40<<10) + "MIDDLE-ONLY-CI-EVIDENCE" + strings.Repeat("b", 40<<10)
	runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: targetFiles}, target: targetFiles, targetSHA: plan.TargetSHA}
	turns := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "navigation-budget", AttemptID: "navigation-budget", Trust: "fixture", MaxOutputTokens: 256, TurnTimeout: time.Second, CILogs: []CILog{{Name: "Validate", Log: log}}, Progress: func(context.Context, string) error { return nil }}
	engine.Turn = func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		turns++
		if !ownerTurnWithinBudget(in) {
			t.Fatalf("owner turn exceeded request budget on turn %d", turns)
		}
		if turns == 1 {
			if len(in.Messages) != 1 {
				t.Fatalf("initial owner messages=%d", len(in.Messages))
			}
			prompt := in.Messages[0].Text
			if !strings.Contains(prompt, "[CI log middle omitted]") || !strings.Contains(prompt, "[Initial index truncated; use list_files and search_files]") || strings.Contains(prompt, "MIDDLE-ONLY-CI-EVIDENCE") {
				t.Fatal("large CI log or path index was not safely summarized")
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "log-middle", Name: "read_ci_log", Arguments: []byte(`{"index":0,"offset":40960,"limit":1024}`)}}}, nil
		}
		found := false
		for _, message := range in.Messages {
			if message.ToolCallID == "log-middle" && strings.Contains(message.Text, "MIDDLE-ONLY-CI-EVIDENCE") {
				found = true
			}
		}
		if !found {
			t.Fatal("read_ci_log could not retrieve omitted evidence")
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "skip", Name: "skip", Arguments: []byte(`{"reason":"fixture complete"}`)}}}, nil
	}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, baseFiles), snapshotForEngine(t, plan.TargetSHA, targetFiles))
	if !errors.Is(err, ErrHandoff) || report.State != "handoff" || turns != 2 {
		t.Fatalf("report=%+v turns=%d error=%v", report, turns, err)
	}
}
