package report

import (
	"github.com/fregateops/vigie/internal/lint"
)

// LintReporter writes lint findings to an output.
type LintReporter interface {
	ReportLint(result lint.Result) error
}
