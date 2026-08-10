package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

func decodeSARIF(t *testing.T, b []byte) sarifRoot {
	t.Helper()
	var root sarifRoot
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("emitted SARIF is not valid JSON: %v", err)
	}
	if root.Version != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", root.Version)
	}
	if len(root.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(root.Runs))
	}
	if root.Runs[0].Tool.Driver.Name != "vigie" {
		t.Errorf("driver name = %q, want vigie", root.Runs[0].Tool.Driver.Name)
	}
	return root
}

func TestSARIFReporter_Report(t *testing.T) {
	results := []runner.SuiteResult{{
		Suite: "deployment",
		Results: []runner.TestResult{
			{TestName: "ok", Pass: true},
			{TestName: "skip", Skipped: true, Pass: true},
			{TestName: "bad", Pass: false, Failures: []string{"want 3, got 1"}},
		},
	}}

	var buf bytes.Buffer
	if err := (&SARIFReporter{Out: &buf}).Report(results); err != nil {
		t.Fatalf("Report: %v", err)
	}
	root := decodeSARIF(t, buf.Bytes())

	run := root.Runs[0]
	// Only the failing test produces a result; pass/skip are excluded.
	if len(run.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(run.Results))
	}
	res := run.Results[0]
	if res.RuleID != "test-failure" || res.Level != "error" {
		t.Errorf("result = {ruleID:%q level:%q}, want test-failure/error", res.RuleID, res.Level)
	}
	if res.Message.Text != "deployment › bad: want 3, got 1" {
		t.Errorf("message = %q", res.Message.Text)
	}
	if len(run.Tool.Driver.Rules) != 1 || run.Tool.Driver.Rules[0].ID != "test-failure" {
		t.Errorf("driver rules = %+v, want one test-failure rule", run.Tool.Driver.Rules)
	}
}

func TestSARIFReporter_ReportLint(t *testing.T) {
	result := lint.Result{Findings: []lint.Finding{
		{RuleID: "chart-yaml_bad", Severity: lint.SeverityError, Message: "boom",
			File: "Chart.yaml", Line: 7, HelpURI: "https://example.test/bad"},
		{RuleID: "tpl_warn", Severity: lint.SeverityWarning, Message: "meh"},
	}}

	var buf bytes.Buffer
	if err := (&SARIFReporter{Out: &buf}).ReportLint(result); err != nil {
		t.Fatalf("ReportLint: %v", err)
	}
	root := decodeSARIF(t, buf.Bytes())
	run := root.Runs[0]

	if len(run.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(run.Results))
	}

	byRule := map[string]sarifResult{}
	for _, r := range run.Results {
		byRule[r.RuleID] = r
	}

	errRes := byRule["chart-yaml_bad"]
	if errRes.Level != "error" {
		t.Errorf("error finding level = %q, want error", errRes.Level)
	}
	if len(errRes.Locations) != 1 {
		t.Fatalf("error finding locations = %d, want 1", len(errRes.Locations))
	}
	loc := errRes.Locations[0].PhysicalLocation
	if loc.ArtifactLocation.URI != "Chart.yaml" {
		t.Errorf("location uri = %q, want Chart.yaml", loc.ArtifactLocation.URI)
	}
	if loc.Region == nil || loc.Region.StartLine != 7 {
		t.Errorf("location region = %+v, want startLine 7", loc.Region)
	}

	warnRes := byRule["tpl_warn"]
	if warnRes.Level != "warning" {
		t.Errorf("warning finding level = %q, want warning", warnRes.Level)
	}
	// A finding with no file must not carry a physical location.
	if len(warnRes.Locations) != 0 {
		t.Errorf("fileless finding has %d locations, want 0", len(warnRes.Locations))
	}

	// helpUri is surfaced on the rule, not the result.
	var found bool
	for _, rule := range run.Tool.Driver.Rules {
		if rule.ID == "chart-yaml_bad" && rule.HelpURI == "https://example.test/bad" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected chart-yaml_bad rule with helpUri; rules = %+v", run.Tool.Driver.Rules)
	}
}

func TestSeverityToSARIFLevel(t *testing.T) {
	cases := map[lint.Severity]string{
		lint.SeverityError:   "error",
		lint.SeverityWarning: "warning",
		lint.SeverityInfo:    "note",
	}
	for sev, want := range cases {
		if got := severityToSARIFLevel(sev); got != want {
			t.Errorf("severityToSARIFLevel(%q) = %q, want %q", sev, got, want)
		}
	}
}
