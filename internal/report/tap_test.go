package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

func TestTAPReporter_Report(t *testing.T) {
	results := []runner.SuiteResult{{
		Suite: "deployment",
		Results: []runner.TestResult{
			{TestName: "renders", Pass: true, Duration: 2 * time.Millisecond},
			{TestName: "skipped one", Skipped: true, Pass: true, Duration: 0},
			{TestName: "bad replicas", Pass: false, Duration: 3 * time.Millisecond,
				Failures: []string{"        → want 3, got 1", "        → second line"}},
		},
	}}

	var buf bytes.Buffer
	rep := &TAPReporter{Out: &buf}
	if err := rep.Report(results); err != nil {
		t.Fatalf("Report: %v", err)
	}
	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if lines[0] != "TAP version 13" {
		t.Errorf("first line = %q, want TAP version 13 header", lines[0])
	}
	if lines[1] != "1..3" {
		t.Errorf("plan line = %q, want 1..3", lines[1])
	}
	for _, want := range []string{
		"ok 1 - deployment › renders",
		"ok 2 - deployment › skipped one (skipped)",
		"not ok 3 - deployment › bad replicas",
		"  #         → want 3, got 1",
		"  #         → second line",
		"  # time=2ms",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
}

func TestTAPReporter_ReportLint(t *testing.T) {
	t.Run("findings", func(t *testing.T) {
		result := lint.Result{Findings: []lint.Finding{
			{RuleID: "chart-yaml_no-name", Message: "missing name", File: "Chart.yaml"},
		}}
		var buf bytes.Buffer
		if err := (&TAPReporter{Out: &buf}).ReportLint(result); err != nil {
			t.Fatalf("ReportLint: %v", err)
		}
		got := buf.String()
		for _, want := range []string{
			"TAP version 13",
			"1..1",
			"not ok 1 - [chart-yaml_no-name] missing name",
			"  # Chart.yaml",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q\n---\n%s", want, got)
			}
		}
	})

	t.Run("no findings", func(t *testing.T) {
		var buf bytes.Buffer
		if err := (&TAPReporter{Out: &buf}).ReportLint(lint.Result{}); err != nil {
			t.Fatalf("ReportLint: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "1..0") || !strings.Contains(got, "# no lint findings") {
			t.Errorf("empty-findings output = %q", got)
		}
	})
}
