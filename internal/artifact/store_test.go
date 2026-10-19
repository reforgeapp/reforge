package artifact

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
)

func TestPrivateArtifactStorageRejectsCredentialsTraversalAndCorruption(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	s, err := NewLocal(nil, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	template := Metadata{ID: domain.NewID(), OrgID: domain.NewID(), RepositoryID: domain.NewID(), TaskID: domain.NewID(), Name: "test.log", MediaType: "text/plain", ExpiresAt: time.Now().Add(time.Hour)}
	for _, payload := range []string{"Authorization: Bearer secret-123456789", "API_KEY=secret-123456789", "-----BEGIN RSA PRIVATE KEY-----", string([]byte{'a', 0, 'b'})} {
		m := template
		m.ID = domain.NewID()
		if err = s.write(context.Background(), &m, strings.NewReader(payload)); !errors.Is(err, ErrContent) {
			t.Fatalf("sensitive or binary artifact accepted: %v", err)
		}
	}
	m := template
	m.Name = "../escape"
	if err = s.write(context.Background(), &m, strings.NewReader("safe")); err == nil {
		t.Fatal("path traversal accepted")
	}
	m = template
	if err = s.write(context.Background(), &m, strings.NewReader(strings.Repeat("x", int(MaxSize)+1))); !errors.Is(err, ErrQuota) {
		t.Fatal("artifact size cap bypassed")
	}
	m = template
	if err = s.write(context.Background(), &m, strings.NewReader("go test ./...\nPASS\n")); err != nil {
		t.Fatal(err)
	}
	file, err := s.Open(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "go test ./...\nPASS\n" {
		t.Fatal("artifact roundtrip failed")
	}
	info, err := os.Stat(filepath.Join(directory, blobName(m.OrgID, m.ID)))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("artifact permissions are not private")
	}
	if err = os.WriteFile(filepath.Join(directory, blobName(m.OrgID, m.ID)), []byte("tampered same size!!!"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Open(m); !errors.Is(err, ErrContent) {
		t.Fatal("corrupted artifact accepted")
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err = os.WriteFile(outside, []byte("host-private"), 0600); err != nil {
		t.Fatal(err)
	}
	m = template
	m.ID = domain.NewID()
	if err = os.Symlink(outside, filepath.Join(directory, blobName(m.OrgID, m.ID))); err != nil {
		t.Fatal(err)
	}
	if err = s.write(context.Background(), &m, strings.NewReader("safe")); err == nil {
		t.Fatal("symlink write accepted")
	}
	if _, err = s.Open(m); err == nil {
		t.Fatal("symlink read accepted")
	}
	host, err := os.ReadFile(outside)
	if err != nil || string(host) != "host-private" {
		t.Fatal("host file was changed")
	}
}

func TestPutTxRejectsInvalidAttemptBeforePreparingBlob(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	storage, err := NewLocal(nil, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	m := Metadata{OrgID: domain.NewID(), RepositoryID: domain.NewID(), TaskID: domain.NewID(), Name: "test.log", MediaType: "text/plain", ExpiresAt: time.Now().Add(time.Hour)}
	if _, err = storage.PutTx(context.Background(), nil, m, "invalid", strings.NewReader("safe")); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("invalid attempt result=%v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid attempt prepared blobs=%v err=%v", entries, err)
	}
}
