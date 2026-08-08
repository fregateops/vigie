package lint

import (
	"fmt"
	"log/slog"

	"github.com/fregateops/vigie/internal/config"
)

// Result is the outcome of linting a single chart.
type Result struct {
	ChartPath string
	Findings  []Finding
}

// HasErrors reports whether any finding has SeverityError.
func (r Result) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Run lints the chart at chartPath using cfg, the supplied ContextBuilder to
// load+render the chart once, and the YAML-driven rule list.
//
// Every HelmLintProvider in the global registry (populated by side-effect
// imports of helmv3, …) contributes its native `helm lint` findings when its
// rule set is enabled in cfg. All findings flow through the same ignore filter.
func Run(chartPath string, cfg *config.LintConfig, builder ContextBuilder, ruleList []Rule) (Result, error) {
	if builder == nil {
		return Result{}, fmt.Errorf("lint: ContextBuilder is required")
	}

	slog.Debug("preparing lint context", "chart", chartPath)
	ctx, err := builder.PrepareContext(chartPath, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("preparing context: %w", err)
	}

	enabled := cfg.EnabledRuleSets()
	enabledNames := make([]string, 0, len(enabled))
	for n := range enabled {
		enabledNames = append(enabledNames, n)
	}
	slog.Debug("enabled rule sets", "sets", enabledNames)

	result := Result{ChartPath: chartPath}

	for _, p := range Providers() {
		set := p.RuleSet()
		if !enabled[set] || cfg.IsRuleDisabled(set) {
			continue
		}
		slog.Debug("running provider", "set", set)
		helmFindings, err := p.LintFindings(chartPath)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", set, err)
		}
		result.Findings = append(result.Findings, applyIgnore(helmFindings, cfg.Ignore)...)
	}

	ranRules := 0
	for _, rule := range ruleList {
		if !enabled[rule.SetName()] || cfg.IsRuleDisabled(rule.ID()) {
			continue
		}
		ranRules++
		findings := rule.Run(ctx)
		if len(findings) > 0 {
			slog.Debug("rule produced findings", "rule", rule.ID(), "count", len(findings))
		}
		result.Findings = append(result.Findings, applyIgnore(findings, cfg.Ignore)...)
	}
	slog.Debug("lint summary", "rulesRan", ranRules, "findings", len(result.Findings))

	return result, nil
}

// applyIgnore removes findings suppressed by ignore rules.
func applyIgnore(findings []Finding, ignores []config.IgnoreRule) []Finding {
	if len(ignores) == 0 {
		return findings
	}
	var out []Finding
	for _, f := range findings {
		if !isIgnored(f, ignores) {
			out = append(out, f)
		}
	}
	return out
}

func isIgnored(f Finding, ignores []config.IgnoreRule) bool {
	for _, ig := range ignores {
		if ig.Rule == f.RuleID {
			if len(ig.Paths) == 0 {
				return true
			}
			for _, p := range ig.Paths {
				if matchGlob(p, f.File) {
					return true
				}
			}
		}
	}
	return false
}

func matchGlob(pattern, path string) bool {
	// Simple suffix glob: "templates/jobs/*.yaml" matches "templates/jobs/migrate.yaml"
	// Exact match only for now; full glob matching can be added later.
	return pattern == path
}
