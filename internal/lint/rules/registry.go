package rules

import (
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/lint/engine"
)

// All returns every built-in lint rule loaded from the embedded YAML rule files.
//
// The "helm-v3-lint" rule set is not represented as Rules — it is dispatched by
// the runner through the registered HelmLintProviders. Everything else is data.
func All() []lint.Rule {
	rules, err := engine.LoadAll()
	if err != nil {
		// Embedded data is built into the binary; a parse failure is a
		// programming error, not a runtime condition.
		panic(err)
	}
	return rules
}
