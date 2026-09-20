package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reforge/internal/deployment"
	"testing"
)

func TestEvidenceCanonicalSigningAndPrivateKeyBoundary(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	document := filepath.Join(dir, "proof.json")
	var public, output bytes.Buffer
	if err := run([]string{"keygen", "--key-file", key}, &public); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(document, []byte(`{"source_sha":"a","artifact_digest":"sha256:b","org_id":"org","repository_id":"repo","build_id":"ci:1","issued_at":"2026-09-21T00:00:00Z","expires_at":"2026-09-22T00:00:00Z"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sign", "--key-file", key, "--kind", "provenance", "--document", document}, &output); err != nil {
		t.Fatal(err)
	}
	var signed deployment.SignedProvenance
	if err := json.Unmarshal(output.Bytes(), &signed); err != nil {
		t.Fatal(err)
	}
	if !deployment.VerifySignature(string(bytes.TrimSpace(public.Bytes())), signed.Document, signed.Signature) {
		t.Fatal("signer incompatible with controller canonical signature")
	}
	signed.Document.ArtifactDigest = "different"
	if deployment.VerifySignature(string(bytes.TrimSpace(public.Bytes())), signed.Document, signed.Signature) {
		t.Fatal("changed artifact authenticated")
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if run([]string{"sign", "--key-file", key, "--kind", "provenance", "--document", document}, &output) == nil {
		t.Fatal("world-readable signing key accepted")
	}
	if run([]string{"keygen", "--key-file", key}, &public) == nil {
		t.Fatal("existing signing key overwritten")
	}
}
