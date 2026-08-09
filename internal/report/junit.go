package report

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

// junitTime formats a duration as fractional seconds, the JUnit XML convention.
func junitTime(d time.Duration) string {
	return fmt.Sprintf("%.3f", d.Seconds())
}

type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct{}

// JUnitReporter writes JUnit XML output.
type JUnitReporter struct {
	Out io.Writer
}

func (r *JUnitReporter) Report(results []runner.SuiteResult) error {
	suites := junitTestSuites{}

	for _, sr := range results {
		suite := junitTestSuite{Name: sr.Suite, Time: junitTime(sr.Duration)}
		for _, tr := range sr.Results {
			suite.Tests++
			tc := junitTestCase{
				Name:      tr.TestName,
				ClassName: sr.Suite,
				Time:      junitTime(tr.Duration),
			}
			if tr.Skipped {
				suite.Skipped++
				tc.Skipped = &junitSkipped{}
			} else if !tr.Pass {
				suite.Failures++
				tc.Failure = &junitFailure{
					Message: "assertion failed",
					Body:    strings.Join(tr.Failures, "\n"),
				}
			}
			suite.TestCases = append(suite.TestCases, tc)
		}
		suites.TestSuites = append(suites.TestSuites, suite)
	}

	out, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JUnit XML: %w", err)
	}
	fmt.Fprintf(r.Out, "%s\n%s\n", xml.Header, out)
	return nil
}

// ReportLint encodes lint findings as a JUnit test suite (each finding = a failed testcase).
func (r *JUnitReporter) ReportLint(result lint.Result) error {
	suite := junitTestSuite{Name: "lint: " + result.ChartPath}
	for _, f := range result.Findings {
		suite.Tests++
		suite.Failures++
		suite.TestCases = append(suite.TestCases, junitTestCase{
			Name:      f.Message,
			ClassName: string(f.Severity),
			Failure: &junitFailure{
				Message: fmt.Sprintf("[%s] %s", f.RuleID, f.Message),
				Body:    f.File,
			},
		})
	}
	suites := junitTestSuites{TestSuites: []junitTestSuite{suite}}
	out, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JUnit XML: %w", err)
	}
	fmt.Fprintf(r.Out, "%s\n%s\n", xml.Header, out)
	return nil
}
