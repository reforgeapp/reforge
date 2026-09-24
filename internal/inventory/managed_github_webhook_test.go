package inventory

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestVerifyManagedSignature(t *testing.T) {
	body := []byte(`{"action":"deleted","installation":{"id":1}}`)
	mac := hmac.New(sha256.New, []byte("secret-value-1234"))
	mac.Write(body)
	signed := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	cases := []struct {
		name   string
		header string
		ok     bool
	}{
		{"valid", signed, true},
		{"missing prefix", hex.EncodeToString(mac.Sum(nil)), false},
		{"wrong length", "sha256=abcd", false},
		{"not hex", "sha256=zzzz", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := verifyManagedSignature("secret-value-1234", http.Header{"X-Hub-Signature-256": {tc.header}}, body); got != tc.ok {
				t.Fatalf("got %v want %v", got, tc.ok)
			}
		})
	}
	if verifyManagedSignature("different-secret-00", http.Header{"X-Hub-Signature-256": {signed}}, body) {
		t.Fatal("wrong secret accepted")
	}
}

func TestManagedIdentity(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		inst, app int64
		ok        bool
	}{
		{"installation only", `{"installation":{"id":7}}`, 7, 0, true},
		{"app id present", `{"installation":{"id":7,"app_id":9}}`, 7, 9, true},
		{"repository scoped", `{"installation":{"id":7},"repository":{"id":9}}`, 7, 0, true},
		{"no installation", `{"repository":{"id":9}}`, 0, 0, false},
		{"zero installation", `{"installation":{"id":0}}`, 0, 0, false},
		{"negative installation", `{"installation":{"id":-1}}`, 0, 0, false},
		{"invalid json", `{`, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst, app, ok := managedIdentity([]byte(tc.body))
			if inst != tc.inst || app != tc.app || ok != tc.ok {
				t.Fatalf("got (%d,%d,%v) want (%d,%d,%v)", inst, app, ok, tc.inst, tc.app, tc.ok)
			}
		})
	}
}

func TestParseManagedEventShape(t *testing.T) {
	cases := []struct {
		name string
		kind string
		body string
		ok   bool
	}{
		{"deleted", "installation", `{"action":"deleted","installation":{"id":7,"account":{"id":9}}}`, true},
		{"suspend", "installation", `{"action":"suspend","installation":{"id":7,"account":{"id":9}}}`, true},
		{"unsuspend", "installation", `{"action":"unsuspend","installation":{"id":7,"account":{"id":9}}}`, true},
		{"new permissions", "installation", `{"action":"new_permissions_accepted","installation":{"id":7,"account":{"id":9}}}`, true},
		{"repositories added", "installation_repositories", `{"action":"added","installation":{"id":7,"account":{"id":9}},"repositories_added":[{"id":1}]}`, true},
		{"repositories removed", "installation_repositories", `{"action":"removed","installation":{"id":7,"account":{"id":9}},"repositories_removed":[{"id":1}]}`, true},
		{"delta list with deleted action is not a revoke", "installation_repositories", `{"action":"deleted","installation":{"id":7,"account":{"id":9}},"repositories_removed":[{"id":1}]}`, true},
		{"repository scoped cannot be lifecycle", "installation", `{"action":"deleted","installation":{"id":7,"account":{"id":9}},"repository":{"id":9}}`, false},
		{"repository scoped delta", "installation_repositories", `{"action":"removed","installation":{"id":7,"account":{"id":9}},"repository":{"id":9},"repositories_removed":[{"id":1}]}`, false},
		{"unknown event kind", "push", `{"action":"deleted","installation":{"id":7,"account":{"id":9}}}`, false},
		{"repository event kind", "repository", `{"action":"deleted","installation":{"id":7,"account":{"id":9}}}`, false},
		{"empty event kind", "", `{"action":"deleted","installation":{"id":7,"account":{"id":9}}}`, false},
		{"installation with delta list", "installation", `{"action":"deleted","installation":{"id":7,"account":{"id":9}},"repositories_removed":[{"id":1}]}`, false},
		{"delta event without lists", "installation_repositories", `{"action":"added","installation":{"id":7,"account":{"id":9}}}`, false},
		{"no installation", "installation", `{"action":"deleted","repository":{"id":9}}`, false},
		{"no action", "installation", `{"installation":{"id":7,"account":{"id":9}}}`, false},
		{"invalid json", "installation", `{`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := parseManagedEvent(tc.kind, []byte(tc.body)); ok != tc.ok {
				t.Fatalf("got %v want %v", ok, tc.ok)
			}
		})
	}
}
