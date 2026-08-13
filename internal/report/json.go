package report

import (
	"encoding/json"
	"io"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

// JSONReporter writes machine-readable results as a single indented JSON
// object. It is the structured counterpart to the pretty reporter, for scripts
// and CI that want to consume results programmatically.
type JSONReporter struct {
	Out io.Writer
}

type jsonTest struct {
	Name       string   `json:"name"`
	Pass       bool     `json:"pass"`
	Skipped    bool     `json:"skipped"`
	SkipReason string   `json:"skipReason,omitempty"`
	Failures   []string `json:"failures,omitempty"`
	DurationMs int64    `json:"durationMs"`
}

type jsonSuite struct {
	File       string     `json:"file"`
	Suite      string     `json:"suite"`
	DurationMs int64      `json:"durationMs"`
	Tests      []jsonTest `json:"tests"`
}

type jsonSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type jsonReport struct {
	Suites  []jsonSuite `json:"suites"`
	Summary jsonSummary `json:"summary"`
}

func (r *JSONReporter) Report(results []runner.SuiteResult) error {
	out := jsonReport{Suites: make([]jsonSuite, 0, len(results))}
	for _, sr := range results {
		js := jsonSuite{
			File:       sr.File,
			Suite:      sr.Suite,
			DurationMs: sr.Duration.Milliseconds(),
			Tests:      make([]jsonTest, 0, len(sr.Results)),
		}
		for _, tr := range sr.Results {
			js.Tests = append(js.Tests, jsonTest{
				Name:       tr.TestName,
				Pass:       tr.Pass,
				Skipped:    tr.Skipped,
				SkipReason: tr.SkipReason,
				Failures:   tr.Failures,
				DurationMs: tr.Duration.Milliseconds(),
			})
			out.Summary.Total++
			switch {
			case tr.Skipped:
				out.Summary.Skipped++
			case tr.Pass:
				out.Summary.Passed++
			default:
				out.Summary.Failed++
			}
		}
		out.Suites = append(out.Suites, js)
	}
	return writeIndentedJSON(r.Out, out)
}

type jsonFinding struct {
	RuleID   string `json:"ruleId"`
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	HelpURI  string `json:"helpUri,omitempty"`
}

func (r *JSONReporter) ReportLint(result lint.Result) error {
	findings := make([]jsonFinding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, jsonFinding{
			RuleID:   f.RuleID,
			Severity: string(f.Severity),
			File:     f.File,
			Line:     f.Line,
			Message:  f.Message,
			HelpURI:  f.HelpURI,
		})
	}
	return writeIndentedJSON(r.Out, map[string]any{
		"findings": findings,
		"summary":  map[string]int{"total": len(findings)},
	})
}

func writeIndentedJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
