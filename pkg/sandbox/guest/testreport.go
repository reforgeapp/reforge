package guest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
)

type TestAttachment struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
}

type TestReport struct {
	Cases       map[string]string
	Attachments []TestAttachment
	Complete    bool
	Failed      bool
}

func NodeFileSuite(name string) bool {
	name = strings.TrimSpace(strings.Split(name, " # ")[0])
	return TestPath(name) && slices.Contains([]string{".js", ".mjs", ".cjs", ".ts", ".mts", ".cts"}, path.Ext(name))
}

func reportJSON(body []byte, target any) bool {
	offset := 0
	for _, line := range bytes.SplitAfter(body, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 && bytes.TrimSpace(line)[0] == '{' {
			if json.Unmarshal(bytes.TrimSpace(body[offset:]), target) == nil {
				return true
			}
		}
		offset += len(line)
	}
	return false
}

func ParseTestReport(format string, body []byte) TestReport {
	out := TestReport{Cases: map[string]string{}}
	if len(body) > 1<<20 {
		return out
	}
	switch format {
	case "jest":
		var report struct {
			NumTotalTests  *int `json:"numTotalTests"`
			NumPassedTests *int `json:"numPassedTests"`
			NumFailedTests *int `json:"numFailedTests"`
			TestResults    []struct {
				Name             string `json:"name"`
				AssertionResults []struct {
					FullName string `json:"fullName"`
					Title    string `json:"title"`
					Status   string `json:"status"`
				} `json:"assertionResults"`
			} `json:"testResults"`
		}
		if !reportJSON(body, &report) || report.NumTotalTests == nil || report.NumPassedTests == nil || report.NumFailedTests == nil {
			return out
		}
		pass, fail, count := 0, 0, 0
		for _, suite := range report.TestResults {
			for _, test := range suite.AssertionResults {
				count++
				title := test.FullName
				if title == "" {
					title = test.Title
				}
				if title == "" {
					return out
				}
				state := ""
				switch test.Status {
				case "passed":
					state = "pass"
					pass++
				case "failed":
					state = "fail"
					fail++
				case "pending", "todo", "skipped", "disabled":
					state = "skip"
				default:
					return out
				}
				key := suite.Name + ":" + title
				if _, exists := out.Cases[key]; exists {
					return out
				}
				out.Cases[key] = state
			}
		}
		out.Complete = count == *report.NumTotalTests && pass == *report.NumPassedTests && fail == *report.NumFailedTests
		out.Failed = fail > 0
	case "playwright":
		var report struct {
			Suites []playwrightSuite                                   `json:"suites"`
			Errors []json.RawMessage                                   `json:"errors"`
			Stats  *struct{ Expected, Skipped, Unexpected, Flaky int } `json:"stats"`
		}
		if !reportJSON(body, &report) || report.Stats == nil {
			return out
		}
		out.Complete = true
		count := 0
		var walk func(playwrightSuite, string)
		walk = func(suite playwrightSuite, prefix string) {
			if suite.Title != "" {
				prefix += suite.Title + "/"
			}
			for _, spec := range suite.Specs {
				for _, test := range spec.Tests {
					count++
					state := "skip"
					if len(test.Results) != 1 || test.ExpectedStatus != "passed" && test.ExpectedStatus != "skipped" {
						out.Complete = false
					}
					if len(test.Results) > 0 {
						result := test.Results[len(test.Results)-1]
						switch result.Status {
						case "passed":
							state = "pass"
						case "failed", "timedOut", "interrupted":
							state = "fail"
						case "skipped":
							state = "skip"
						default:
							out.Complete = false
						}
						out.Attachments = append(out.Attachments, result.Attachments...)
					}
					title := spec.Title
					if title == "" {
						out.Complete = false
					}
					key := fmt.Sprintf("%s%s/%s:%s", prefix, spec.File, test.ProjectName, title)
					if _, exists := out.Cases[key]; exists {
						out.Complete = false
					}
					out.Cases[key] = state
					if state == "fail" {
						out.Failed = true
					}
				}
			}
			for _, child := range suite.Suites {
				walk(child, prefix)
			}
		}
		for _, suite := range report.Suites {
			walk(suite, "")
		}
		out.Complete = out.Complete && count == report.Stats.Expected+report.Stats.Skipped+report.Stats.Unexpected+report.Stats.Flaky && len(report.Errors) == 0 && report.Stats.Flaky == 0
		out.Failed = out.Failed || report.Stats.Unexpected > 0 || len(report.Errors) > 0
	}
	return out
}

type playwrightSuite struct {
	Title  string            `json:"title"`
	Suites []playwrightSuite `json:"suites"`
	Specs  []struct {
		Title string `json:"title"`
		File  string `json:"file"`
		Tests []struct {
			ProjectName    string `json:"projectName"`
			ExpectedStatus string `json:"expectedStatus"`
			Results        []struct {
				Status      string           `json:"status"`
				Attachments []TestAttachment `json:"attachments"`
			} `json:"results"`
		} `json:"tests"`
	} `json:"specs"`
}

func NamedPasses(cases map[string]string) int {
	n := 0
	for name, state := range cases {
		if strings.TrimSpace(name) != "" && state == "pass" {
			n++
		}
	}
	return n
}
