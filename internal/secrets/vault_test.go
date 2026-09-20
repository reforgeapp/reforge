package secrets

import (
	"bytes"
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
