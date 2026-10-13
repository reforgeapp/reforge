package guest

import "testing"

func TestNamedFrameworkReportsRejectIncompleteAndCountRealCases(t *testing.T) {
	cases := []struct {
		name, format, body string
		complete, failed   bool
		passes             int
	}{
		{"jest actual", "jest", `{"numTotalTests":2,"numPassedTests":1,"numFailedTests":1,"testResults":[{"name":"checkout.test.js","assertionResults":[{"fullName":"checkout desktop","status":"passed"},{"fullName":"checkout mobile","status":"failed"}]}]}`, true, true, 1},
		{"empty", "jest", `{"numTotalTests":0,"numPassedTests":0,"numFailedTests":0,"testResults":[]}`, true, false, 0},
		{"skipped", "jest", `{"numTotalTests":1,"numPassedTests":0,"numFailedTests":0,"testResults":[{"name":"checkout.test.js","assertionResults":[{"fullName":"checkout","status":"pending"}]}]}`, true, false, 0},
		{"miscount", "jest", `{"numTotalTests":3,"numPassedTests":3,"numFailedTests":0,"testResults":[]}`, false, false, 0},
		{"unreported", "jest", `{"success":true}`, false, false, 0},
		{"browser actual", "playwright", `{"suites":[{"title":"shop","specs":[{"title":"Checkout mobile","file":"checkout.spec.ts","tests":[{"projectName":"mobile","expectedStatus":"passed","results":[{"status":"passed","attachments":[{"name":"checkout","path":"/workspace/.reforge/browser/js-01/checkout.png","contentType":"image/png"}]}]}]}]}],"errors":[],"stats":{"expected":1,"unexpected":0,"skipped":0,"flaky":0}}`, true, false, 1},
		{"browser failure", "playwright", `{"suites":[{"specs":[{"title":"Checkout mobile","file":"checkout.spec.ts","tests":[{"projectName":"mobile","expectedStatus":"passed","results":[{"status":"failed"}]}]}]}],"errors":[],"stats":{"expected":0,"unexpected":1,"skipped":0,"flaky":0}}`, true, true, 0},
		{"browser empty success", "playwright", `{"suites":[],"errors":[],"stats":{"expected":0,"unexpected":0,"skipped":0,"flaky":0}}`, true, false, 0},
		{"browser retry", "playwright", `{"suites":[{"specs":[{"title":"Checkout mobile","file":"checkout.spec.ts","tests":[{"projectName":"mobile","expectedStatus":"passed","results":[{"status":"failed"},{"status":"passed"}]}]}]}],"errors":[],"stats":{"expected":0,"unexpected":0,"skipped":0,"flaky":1}}`, false, false, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseTestReport(c.format, []byte("npm startup\n"+c.body))
			if got.Complete != c.complete || got.Failed != c.failed || NamedPasses(got.Cases) != c.passes {
				t.Fatalf("report: %+v", got)
			}
		})
	}
}
