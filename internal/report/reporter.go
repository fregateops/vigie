package report

import (
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

// Reporter writes test results to an output.
type Reporter interface {
	Report(results []runner.SuiteResult) error
}

// LintReporter writes lint findings to an output.
type LintReporter interface {
	ReportLint(result lint.Result) error
}
