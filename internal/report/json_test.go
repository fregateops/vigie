package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

func TestJSONReporter_Report(t *testing.T) {
	results := []runner.SuiteResult{
		{
			File:     "tests/unit/a_test.yaml",
			Suite:    "a",
			Duration: 5 * time.Millisecond,
			Results: []runner.TestResult{
				{SuiteName: "a", TestName: "passes", Pass: true, Duration: time.Millisecond},
				{SuiteName: "a", TestName: "fails", Pass: false, Failures: []string{"boom"}, Duration: 2 * time.Millisecond},
				{SuiteName: "a", TestName: "skipped", Pass: true, Skipped: true, SkipReason: "nope"},
			},
		},
	}

	var buf bytes.Buffer
	rep := &JSONReporter{Out: &buf}
	if err := rep.Report(results); err != nil {
		t.Fatalf("Report: %v", err)
	}

	var got struct {
		Suites []struct {
			File  string `json:"file"`
			Suite string `json:"suite"`
			Tests []struct {
				Name     string   `json:"name"`
				Pass     bool     `json:"pass"`
				Skipped  bool     `json:"skipped"`
				Failures []string `json:"failures"`
			} `json:"tests"`
		} `json:"suites"`
		Summary struct {
			Total   int `json:"total"`
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Skipped int `json:"skipped"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, buf.String())
	}

	if got.Summary != (struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Skipped int `json:"skipped"`
	}{Total: 3, Passed: 1, Failed: 1, Skipped: 1}) {
		t.Errorf("summary = %+v, want total=3 passed=1 failed=1 skipped=1", got.Summary)
	}
	if len(got.Suites) != 1 || len(got.Suites[0].Tests) != 3 {
		t.Fatalf("expected 1 suite with 3 tests, got %+v", got.Suites)
	}

	var sawFail bool
	for _, tc := range got.Suites[0].Tests {
		if tc.Name == "fails" {
			sawFail = true
			if tc.Pass || len(tc.Failures) == 0 {
				t.Errorf("failing test should be pass=false with failures, got %+v", tc)
			}
		}
	}
	if !sawFail {
		t.Error("failing test not present in output")
	}
}

func TestJSONReporter_ReportLint(t *testing.T) {
	result := lint.Result{Findings: []lint.Finding{
		{RuleID: "chart-yaml_missing-name", Severity: lint.SeverityError, File: "Chart.yaml", Line: 3, Message: "name is required"},
	}}

	var buf bytes.Buffer
	rep := &JSONReporter{Out: &buf}
	if err := rep.ReportLint(result); err != nil {
		t.Fatalf("ReportLint: %v", err)
	}

	var got struct {
		Findings []struct {
			RuleID   string `json:"ruleId"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Line     int    `json:"line"`
		} `json:"findings"`
		Summary struct {
			Total int `json:"total"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, buf.String())
	}

	if got.Summary.Total != 1 || len(got.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %+v", got)
	}
	if got.Findings[0].RuleID != "chart-yaml_missing-name" || got.Findings[0].Severity != "error" || got.Findings[0].Line != 3 {
		t.Errorf("finding mismatch: %+v", got.Findings[0])
	}
}
