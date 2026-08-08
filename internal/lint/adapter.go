package lint

import "github.com/fregateops/vigie/internal/config"

// ContextBuilder loads a chart and prepares the lint Context.
//
// Currently only the v3 implementation exists; a v4 builder can be added later
// without touching rule logic. The runner uses exactly one builder per run.
type ContextBuilder interface {
	PrepareContext(chartPath string, cfg *config.LintConfig) (Context, error)
}

// HelmLintProvider exposes a specific Helm version's native lint output as
// Findings. Each implementation is bound to a single rule set name (e.g.
// "helm-v3-lint") so the runner can dispatch by ruleSet — providers whose rule
// set is not enabled are skipped.
type HelmLintProvider interface {
	RuleSet() string
	LintFindings(chartPath string) ([]Finding, error)
}

// registeredProviders holds every HelmLintProvider that has been declared via
// RegisterProvider, in registration order. Each Helm-version subpackage
// (helmv3, …) registers itself in its init(); the runner consumes the full set
// and filters by enabled rule sets.
var registeredProviders []HelmLintProvider

// RegisterProvider adds p to the global provider registry. Intended to be
// called from package init() in each Helm-version subpackage.
func RegisterProvider(p HelmLintProvider) {
	registeredProviders = append(registeredProviders, p)
}

// Providers returns the current provider registry. The slice is a copy so
// callers may not mutate the registry through it.
func Providers() []HelmLintProvider {
	out := make([]HelmLintProvider, len(registeredProviders))
	copy(out, registeredProviders)
	return out
}
