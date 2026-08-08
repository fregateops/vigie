package engine

import "github.com/fregateops/vigie/internal/lint"

// YAMLRule is a lint.Rule implementation backed by a declarative RuleDef.
// All YAML-driven rules share this single Go type — only the data differs.
type YAMLRule struct {
	ruleSet string
	def     RuleDef
}

// ID is composed as "{ruleSet}_{def.id}" so every rule ID self-describes
// which rule set it belongs to. The YAML carries only the short name.
func (r *YAMLRule) ID() string                     { return r.ruleSet + "_" + r.def.ID }
func (r *YAMLRule) SetName() string                { return r.ruleSet }
func (r *YAMLRule) DefaultSeverity() lint.Severity { return lint.Severity(r.def.Severity) }

func (r *YAMLRule) Run(ctx lint.Context) []lint.Finding {
	id := r.ID()
	switch r.def.Scope {
	case "chart":
		return runChartScope(ctx, id, r.def)
	case "document":
		return runDocumentScope(ctx, id, r.def)
	case "container":
		return runContainerScope(ctx, id, r.def)
	case "removedAPI":
		return runRemovedAPIScope(ctx, id, r.def)
	default:
		return nil
	}
}
