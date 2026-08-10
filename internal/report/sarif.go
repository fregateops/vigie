package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
	"github.com/fregateops/vigie/internal/version"
)

// SARIFReporter writes SARIF 2.1.0 output.
// It implements Reporter for both test results (Report) and lint findings (ReportLint).
type SARIFReporter struct {
	Out io.Writer
}

// SARIF 2.1.0 types.

type sarifRoot struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string     `json:"id"`
	Name             string     `json:"name,omitempty"`
	ShortDescription *sarifText `json:"shortDescription,omitempty"`
	HelpURI          string     `json:"helpUri,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

func newSARIFRoot(results []sarifResult, rules []sarifRule) sarifRoot {
	return sarifRoot{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "vigie",
					Version:        version.Version,
					InformationURI: "https://github.com/fregateops/vigie",
					Rules:          rules,
				},
			},
			Results: results,
		}},
	}
}

func writeSARIF(w io.Writer, root sarifRoot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(root)
}

// Report writes test failures as SARIF results.
func (r *SARIFReporter) Report(results []runner.SuiteResult) error {
	var sarifResults []sarifResult
	ruleSet := map[string]bool{}

	for _, sr := range results {
		for _, tr := range sr.Results {
			if tr.Pass || tr.Skipped {
				continue
			}
			ruleID := "test-failure"
			ruleSet[ruleID] = true
			for _, f := range tr.Failures {
				sarifResults = append(sarifResults, sarifResult{
					RuleID: ruleID,
					Level:  "error",
					Message: sarifText{
						Text: fmt.Sprintf("%s › %s: %s", sr.Suite, tr.TestName, f),
					},
				})
			}
		}
	}

	var rules []sarifRule
	if ruleSet["test-failure"] {
		rules = append(rules, sarifRule{
			ID:               "test-failure",
			ShortDescription: &sarifText{Text: "Helm template assertion failed"},
		})
	}

	return writeSARIF(r.Out, newSARIFRoot(sarifResults, rules))
}

// ReportLint writes lint findings as SARIF results.
func (r *SARIFReporter) ReportLint(result lint.Result) error {
	ruleIndex := map[string]sarifRule{}
	var sarifResults []sarifResult

	for _, f := range result.Findings {
		if _, ok := ruleIndex[f.RuleID]; !ok {
			sr := sarifRule{ID: f.RuleID}
			if f.HelpURI != "" {
				sr.HelpURI = f.HelpURI
			}
			ruleIndex[f.RuleID] = sr
		}

		level := severityToSARIFLevel(f.Severity)
		res := sarifResult{
			RuleID:  f.RuleID,
			Level:   level,
			Message: sarifText{Text: f.Message},
		}
		if f.File != "" {
			loc := sarifLocation{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
				},
			}
			if f.Line > 0 {
				loc.PhysicalLocation.Region = &sarifRegion{StartLine: f.Line}
			}
			res.Locations = append(res.Locations, loc)
		}
		sarifResults = append(sarifResults, res)
	}

	rules := make([]sarifRule, 0, len(ruleIndex))
	for _, r := range ruleIndex {
		rules = append(rules, r)
	}

	return writeSARIF(r.Out, newSARIFRoot(sarifResults, rules))
}

func severityToSARIFLevel(s lint.Severity) string {
	switch s {
	case lint.SeverityError:
		return "error"
	case lint.SeverityWarning:
		return "warning"
	default:
		return "note"
	}
}
