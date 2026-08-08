package lint

import (
	"github.com/fregateops/vigie/internal/config"
	"helm.sh/helm/v3/pkg/chart"
)

// Severity classifies how serious a finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding is a single lint violation.
type Finding struct {
	RuleID   string
	Severity Severity
	File     string
	Line     int
	Message  string
	HelpURI  string
}

// Context carries everything a lint rule needs to inspect.
type Context struct {
	ChartPath string
	Chart     *chart.Chart
	// ChartMeta is chart metadata as a plain map[string]any, suitable for
	// CEL bindings. Populated by the ContextBuilder when the context is built.
	ChartMeta    map[string]any
	RenderedDocs []map[string]any
	// KubeVersion is the target Kubernetes version (e.g. "1.27") used by
	// version-aware rules such as the deprecation table. Empty means
	// "unspecified" — version-filtered rules treat this as "report all".
	KubeVersion string
	Cfg         *config.LintConfig
}
