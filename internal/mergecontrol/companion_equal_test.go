package mergecontrol

import "testing"

func TestCompanionsEqualPreservesEmptySetSemantics(t *testing.T) {
	if !companionsEqual(nil, []Companion{}) || !companionsEqual([]Companion{}, nil) {
		t.Fatal("empty companion sets should compare equal")
	}
	if companionsEqual([]Companion{{TaskID: "a"}}, []Companion{{TaskID: "b"}}) {
		t.Fatal("different companion proofs compared equal")
	}
}
