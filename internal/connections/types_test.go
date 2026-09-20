package connections

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

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
