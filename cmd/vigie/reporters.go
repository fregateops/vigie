package main

import (
	"io"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/report"
)

// selectReporter maps the --output format to a reporter. Every reporter
// handles both test results and lint findings, so the same switch serves
// `vigie test` and `vigie lint`. sarif and tap land in M4; unknown formats
// fall back to pretty.
func selectReporter(format string, out io.Writer, ciKind cienv.Kind) report.Reporter {
	switch format {
	case "junit":
		return &report.JUnitReporter{Out: out}
	default:
		return &report.PrettyReporter{Out: out, CI: ciKind}
	}
}
