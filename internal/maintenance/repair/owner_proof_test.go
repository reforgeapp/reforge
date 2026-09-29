package repair

import "testing"

func TestLostPassingCase(t *testing.T) {
	base := []CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "pass", "3:flaky": "fail"}}}
	for name, tc := range map[string]struct {
		candidate []CheckResult
		lost      bool
	}{
		"kept":        {[]CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "pass", "3:flaky": "pass"}}}, false},
		"renumbered":  {[]CheckResult{{Cases: map[string]string{"1:new": "pass", "2:adds": "pass", "3:subtracts": "pass"}}}, false},
		"deleted":     {[]CheckResult{{Cases: map[string]string{"1:adds": "pass"}}}, true},
		"now-failing": {[]CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "fail"}}}, true},
	} {
		if got := lostPassingCase(base, tc.candidate); got != tc.lost {
			t.Errorf("%s: lost=%t", name, got)
		}
	}
}
