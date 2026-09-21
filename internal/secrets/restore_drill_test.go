package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

func TestRestoreDrillKeyRecovery(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	other := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	scope := Binding{OrgID: "00000000-0000-4000-8000-000000000001", ConnectionID: "00000000-0000-4000-8000-000000000002", Version: 3}
	source, err := New("primary", map[string]string{"primary": key})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := source.Seal(scope, []byte("restored-credential"))
	if err != nil {
		t.Fatal(err)
	}
	backup, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Envelope
	if err = json.Unmarshal(backup, &persisted); err != nil {
		t.Fatal(err)
	}
	restored, err := New("primary", map[string]string{"primary": key})
	if err != nil {
		t.Fatal(err)
	}
	value, err := restored.Open(scope, persisted)
	if err != nil || string(value) != "restored-credential" {
		t.Fatalf("restored key must decrypt the backed-up envelope: %q %v", value, err)
	}
	wrong, err := New("primary", map[string]string{"primary": other})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Open(scope, persisted); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a different key must not decrypt the envelope, got %v", err)
	}
}
