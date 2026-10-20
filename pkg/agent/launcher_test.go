package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContainerLauncherPinsImageAndRecordsArgv(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	docker := filepath.Join(dir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(docker, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	image := "ghcr.io/example/codex@sha256:" + strings.Repeat("a", 64)
	launcher := NewContainerLauncher(ContainerConfig{Docker: docker, Image: image, Executable: "/app/codex", Args: []string{"app-server"}, Environment: []string{"LANG=C"}})
	if launcher == nil {
		t.Fatal("valid pinned container launcher was rejected")
	}
	runtime, err := launcher(context.Background(), Binding{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Transport.Close()
	var raw []byte
	for i := 0; i < 40; i++ {
		if raw, err = os.ReadFile(argsFile); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"run", "--network", "none", "--read-only", "--user", "65534:65534", image, "/app/codex", "app-server", "LANG=C"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("container argv missing %q: %q", want, raw)
		}
	}
}

func TestContainerLauncherRejectsUnpinnedImageAndSecrets(t *testing.T) {
	if NewContainerLauncher(ContainerConfig{Image: "ghcr.io/example/codex:latest", Executable: "/app/codex"}) != nil {
		t.Fatal("mutable image tag accepted")
	}
	image := "ghcr.io/example/codex@sha256:" + strings.Repeat("a", 64)
	if NewContainerLauncher(ContainerConfig{Image: image, Executable: "/app/codex", Environment: []string{"OPENAI_API_KEY=secret"}}) != nil {
		t.Fatal("credential environment accepted")
	}
	if NewContainerLauncher(ContainerConfig{Image: image, Executable: "codex"}) != nil {
		t.Fatal("relative executable accepted")
	}
}

func TestCommandLauncherEnforcesDigest(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "runtime")
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(binary, body, 0755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	mismatch := NewCommandLauncher(CommandConfig{Executable: binary, ExpectedSHA256: strings.Repeat("0", 64)})
	if _, err := mismatch(context.Background(), Binding{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("digest mismatch accepted: %v", err)
	}
	launcher := NewCommandLauncher(CommandConfig{Executable: binary, ExpectedSHA256: hex.EncodeToString(sum[:])})
	runtime, err := launcher(context.Background(), Binding{})
	if err != nil {
		t.Fatal(err)
	}
	_ = runtime.Transport.Close()
}
