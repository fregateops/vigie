package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

// tapVersion is the TAP v13 preamble line. It must be the very first line of
// the stream; without it consumers fall back to TAP12 parsing.
const tapVersion = "TAP version 13"

func tapTime(d time.Duration) string {
	return fmt.Sprintf("# time=%dms", d.Milliseconds())
}

// TAPReporter writes TAP (Test Anything Protocol) v13 output.
type TAPReporter struct {
	Out io.Writer
}

func (r *TAPReporter) Report(results []runner.SuiteResult) error {
	// Count total tests for the plan line.
	total := 0
	for _, sr := range results {
		total += len(sr.Results)
	}

	fmt.Fprintln(r.Out, tapVersion)
	fmt.Fprintf(r.Out, "1..%d\n", total)

	n := 0
	for _, sr := range results {
		for _, tr := range sr.Results {
			n++
			label := fmt.Sprintf("%s › %s", sr.Suite, tr.TestName)

			switch {
			case tr.Skipped:
				fmt.Fprintf(r.Out, "ok %d - %s (skipped)\n", n, label)
			case tr.Pass:
				fmt.Fprintf(r.Out, "ok %d - %s\n", n, label)
			default:
				fmt.Fprintf(r.Out, "not ok %d - %s\n", n, label)
				for _, f := range tr.Failures {
					// Prefix each line with "  # " as TAP diagnostic lines.
					for _, line := range strings.Split(f, "\n") {
						fmt.Fprintf(r.Out, "  # %s\n", line)
					}
				}
			}
			fmt.Fprintf(r.Out, "  %s\n", tapTime(tr.Duration))
		}
	}

	return nil
}

// ReportLint writes lint findings in TAP format.
func (r *TAPReporter) ReportLint(result lint.Result) error {
	fmt.Fprintln(r.Out, tapVersion)
	fmt.Fprintf(r.Out, "1..%d\n", len(result.Findings))
	for i, f := range result.Findings {
		fmt.Fprintf(r.Out, "not ok %d - [%s] %s\n", i+1, f.RuleID, f.Message)
		if f.File != "" {
			fmt.Fprintf(r.Out, "  # %s\n", f.File)
		}
	}
	if len(result.Findings) == 0 {
		fmt.Fprintf(r.Out, "# no lint findings\n")
	}
	return nil
}
