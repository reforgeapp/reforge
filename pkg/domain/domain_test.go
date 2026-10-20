package domain

import (
	"regexp"
	"testing"
)

func TestStableIDIsDeterministicUUID(t *testing.T) {
	a, b := StableID("merge", "finding", "7"), StableID("merge", "finding", "7")
	if a != b || a == StableID("merge", "finding", "8") || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(a) {
		t.Fatalf("stable id %s %s", a, b)
	}
}
