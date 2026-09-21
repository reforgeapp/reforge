package connections

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOfficialAgentProvidersAreDistinct(t *testing.T) {
	base := CreateRequest{Kind: "agent", Name: "runtime", Endpoint: "https://runtime.example", Settings: Settings{AuthKind: "official_runtime", BillingRoute: "subscription"}}
	for _, provider := range []string{"codex", "claude_code", "agy", "gemini_cli"} {
		request := base
		request.Provider = provider
		if !validSetup(request) {
			t.Fatalf("provider %q should be accepted for an official agent connection", provider)
		}
	}
	for _, provider := range []string{"gemini", "antigravity", "codex_cli", ""} {
		request := base
		request.Provider = provider
		if validSetup(request) {
			t.Fatalf("provider %q must not be accepted", provider)
		}
	}
}

func TestWriteOnlyCredential(t *testing.T) {
	const secret = "credential-must-never-be-emitted"
	var input CreateRequest
	if err := json.Unmarshal([]byte(`{"secret":"`+secret+`"}`), &input); err != nil || input.Secret != secret {
		t.Fatal("write credential decode failed")
	}
	for _, value := range []any{input, Resolved{Secret: secret}} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), secret) || strings.Contains(fmt.Sprintf("%+v %#v", value, value), secret) {
			t.Fatal("credential serialized")
		}
	}
}
