package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/report"
	"github.com/fregateops/vigie/internal/runner"
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

// noteUncheckedKinds tells the user which kinds passed kubeconform without a
// schema to check them against. It is a note, not a warning: charts shipping
// CRDs would otherwise never exit 0 without extra schema locations.
func noteUncheckedKinds(results []runner.SuiteResult) {
	if kinds := runner.UncheckedKinds(results); len(kinds) > 0 {
		fmt.Fprintf(os.Stderr, "vigie: note: no schema found for %s; these were not validated (add a location under schemas.locations, e.g. the CRDs catalog)\n", strings.Join(kinds, ", "))
	}
}
