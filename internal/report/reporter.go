package report

import (
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

// Reporter renders results to an output. Every reporter handles both test
// results (Report) and lint findings (ReportLint) so a single --output format
// works uniformly across `vigie test` and `vigie lint`.
type Reporter interface {
	Report(results []runner.SuiteResult) error
	ReportLint(result lint.Result) error
}
