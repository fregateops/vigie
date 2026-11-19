package lint

import (
	"errors"
	"testing"

	"github.com/fregateops/vigie/internal/config"
)

func TestIsIgnored_PathsAreGlobs(t *testing.T) {
	const rule = "template-best-practices_missing-resource-limits"
	ignores := []config.IgnoreRule{{Rule: rule, Paths: []string{"templates/jobs/*.yaml"}}}

	if !isIgnored(Finding{RuleID: rule, File: "templates/jobs/migrate.yaml"}, ignores) {
		t.Error("finding in templates/jobs/migrate.yaml should be ignored")
	}
	if isIgnored(Finding{RuleID: rule, File: "templates/deployment.yaml"}, ignores) {
		t.Error("finding outside the glob must not be ignored")
	}
}

type fixedContext struct{ ctx Context }

func (f fixedContext) PrepareContext(string, *config.LintConfig) (Context, error) { return f.ctx, nil }

func TestRun_RenderFailureIsAFinding(t *testing.T) {
	builder := fixedContext{Context{RenderErr: errors.New("foo is required")}}
	res, err := Run("chart", &config.LintConfig{}, builder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].RuleID != RenderFailedRuleID || !res.HasErrors() {
		t.Fatalf("want one render-failed error finding, got %+v", res.Findings)
	}
}
