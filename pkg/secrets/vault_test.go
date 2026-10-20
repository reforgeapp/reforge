package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
)

func TestEnvelopeBindingAndRotation(t *testing.T) {
	old := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	next := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	v, err := New("old", map[string]string{"old": old})
	if err != nil {
		t.Fatal(err)
	}
	b := Binding{OrgID: "org-a", ConnectionID: "connection-a", Version: 1}
	e, err := v.Seal(b, []byte("private-api-key"))
	if err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []Binding{{"org-b", "connection-a", 1}, {"org-a", "connection-b", 1}, {"org-a", "connection-a", 2}} {
		if _, err := v.Open(wrong, e); err == nil {
			t.Fatal("credential accepted wrong tenant/connection/version")
		}
	}
	tampered := e
	tampered.Ciphertext = bytes.Clone(e.Ciphertext)
	tampered.Ciphertext[0] ^= 1
	if _, err := v.Open(b, tampered); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	rotated, err := New("next", map[string]string{"old": old, "next": next})
	if err != nil {
		t.Fatal(err)
	}
	rewrapped, err := rotated.Rewrap(b, e)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(e.Ciphertext, rewrapped.Ciphertext) {
		t.Fatal("rewrapping changed credential ciphertext")
	}
	newOnly, _ := New("next", map[string]string{"next": next})
	plain, err := newOnly.Open(b, rewrapped)
	if err != nil || string(plain) != "private-api-key" {
		t.Fatal("restored rotated credential unavailable")
	}
	if _, err := v.Open(b, rewrapped); err == nil {
		t.Fatal("old wrapping key decrypted rotated envelope")
	}
	if _, err := newOnly.Open(b, e); err == nil {
		t.Fatal("missing wrapping key accepted")
	}
}

func TestRecordsExceedCredentialLimit(t *testing.T) {
	v, err := New("k", map[string]string{"k": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))})
	if err != nil {
		t.Fatal(err)
	}
	b := Binding{OrgID: "org-a", ConnectionID: "turn-a", Version: 1}
	record := bytes.Repeat([]byte("x"), 100<<10)
	if _, err = v.SealContext(context.Background(), b, record); err == nil {
		t.Fatal("credential seal accepted a record-sized value")
	}
	e, err := v.SealRecord(context.Background(), b, record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v.OpenContext(context.Background(), b, e); err == nil {
		t.Fatal("credential open accepted a record-sized value")
	}
	if got, err := v.OpenRecord(context.Background(), b, e); err != nil || !bytes.Equal(got, record) {
		t.Fatalf("record round trip: %v", err)
	}
}
