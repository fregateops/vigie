package main

import (
	"log/slog"
	"os"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/lint/helmv3"
	"github.com/fregateops/vigie/internal/lint/rules"
	"github.com/fregateops/vigie/internal/report"
	"github.com/spf13/cobra"
)

var (
	flagLintRuleSets     []string
	flagLintDisableRules []string
	flagLintKubeVersion  string
)

var lintCmd = &cobra.Command{
	Use:   "lint <chart>",
	Short: "Static analysis: chart-yaml, deprecations, best-practices",
	Example: `  # Lint a chart with all default rule sets
  vigie lint ./mychart

  # Run only best-practices and skip one rule, emitting JUnit
  vigie lint ./mychart --rule-sets template-best-practices \
    --disable-rules template-best-practices_missing-resource-limits -o junit`,
	Args: cobra.ExactArgs(1),
	RunE: runLintCmd,
}

func init() {
	lintCmd.Flags().StringSliceVar(&flagLintRuleSets, "rule-sets", nil,
		"Comma-separated rule sets to run (overrides lint.ruleSets in .vigie.yaml). "+
			"Available: helm-v3-lint, chart-yaml, template-best-practices, deprecation")
	lintCmd.Flags().StringSliceVar(&flagLintDisableRules, "disable-rules", nil,
		"Comma-separated rule IDs to skip (added to lint.disableRules from .vigie.yaml)")
	lintCmd.Flags().StringVar(&flagLintKubeVersion, "kube-version", "",
		"Target Kubernetes API version for deprecation checks (overrides lint.kubeVersions in .vigie.yaml)")
	rootCmd.AddCommand(lintCmd)
}

func runLintCmd(cmd *cobra.Command, args []string) error {
	chartPath := args[0]

	slog.Debug("invoked", "command", "lint", "chart", chartPath)

	cfg, err := config.Load(chartPath)
	if err != nil {
		exitErr(3, "loading config: %v", err)
	}

	// CLI --rule-sets overrides config; --disable-rules is additive.
	if len(flagLintRuleSets) > 0 {
		cfg.Lint.RuleSets = flagLintRuleSets
	}
	if len(flagLintDisableRules) > 0 {
		cfg.Lint.DisableRules = append(cfg.Lint.DisableRules, flagLintDisableRules...)
	}
	if flagLintKubeVersion != "" {
		cfg.Lint.KubeVersions = []string{flagLintKubeVersion}
	}

	result, err := lint.Run(chartPath, &cfg.Lint, helmv3.New(), rules.All())
	if err != nil {
		exitErr(2, "lint: %v", err)
	}

	rep := &report.PrettyReporter{Out: os.Stdout, CI: cienv.Detect()}
	if err := rep.ReportLint(result); err != nil {
		exitErr(2, "reporting: %v", err)
	}

	if result.HasErrors() {
		os.Exit(1)
	}
	return nil
}
