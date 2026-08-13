package main

import (
	"io"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/report"
)

// selectReporter maps the --output format to a reporter. Every reporter
// handles both test results and lint findings, so the same switch serves
// `vigie test`, `vigie validate`, and `vigie lint`. Unknown formats fall back
// to pretty.
func selectReporter(format string, out io.Writer, ciKind cienv.Kind) report.Reporter {
	switch format {
	case "json":
		return &report.JSONReporter{Out: out}
	case "junit":
		return &report.JUnitReporter{Out: out}
	case "sarif":
		return &report.SARIFReporter{Out: out}
	case "tap":
		return &report.TAPReporter{Out: out}
	default:
		return &report.PrettyReporter{Out: out, CI: ciKind}
	}
}
