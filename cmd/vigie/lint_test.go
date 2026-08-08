package main

import (
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/lint/helmv3"
	"github.com/fregateops/vigie/internal/lint/rules"
)

// runLint exercises the same stack as the lint command: default config, the
// helm-v3 context builder, and the embedded rule sets.
func runLint(t *testing.T, chartPath string) lint.Result {
	t.Helper()
	cfg, err := config.Load(chartPath)
	if err != nil {
		t.Fatalf("config.Load(%s): %v", chartPath, err)
	}
	result, err := lint.Run(chartPath, &cfg.Lint, helmv3.New(), rules.All())
	if err != nil {
		t.Fatalf("lint.Run(%s): %v", chartPath, err)
	}
	return result
}

func TestLint_CleanChart_NoErrors(t *testing.T) {
	result := runLint(t, "../../testdata/charts/clean")
	if result.HasErrors() {
		t.Fatalf("clean chart should produce no error-severity findings, got: %+v", result.Findings)
	}
}

func TestLint_BrokenChart_ReportsExpectedFindings(t *testing.T) {
	result := runLint(t, "../../testdata/charts/lint-issues")
	if !result.HasErrors() {
		t.Fatal("lint-issues chart should produce error-severity findings, got none")
	}

	// The broken chart deliberately trips a rule from each error-producing set:
	// a removed API version (deprecation) and a hardcoded namespace
	// (template-best-practices).
	wantRuleSubstrings := []string{"hardcoded-namespace", "deprecation_"}
	for _, want := range wantRuleSubstrings {
		found := false
		for _, f := range result.Findings {
			if strings.Contains(f.RuleID, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a finding whose rule ID contains %q; findings: %+v", want, result.Findings)
		}
	}
}
