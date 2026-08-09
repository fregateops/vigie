package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/runner"
	"github.com/spf13/cobra"
)

var (
	flagTestFile          string
	flagTestTestsDir      string
	flagTestSnapshotDir   string
	flagTestPassOnWarning bool
)

// exitWarnings is the exit code for a run that produced only warnings (e.g. no
// tests executed) — distinct from setup (2)/user (3) errors so CI can tell
// "your chart has no tests" apart from "the tool broke". Mirrors pytest's
// dedicated "no tests collected" code.
const exitWarnings = 5

var testCmd = &cobra.Command{
	Use:   "test <chart>",
	Short: "Render templates per test and run user assertions",
	Example: `  # Run every tests/unit/*_test.yaml under the chart
  vigie test ./mychart

  # Run a single test file and emit JUnit for CI
  vigie test ./mychart --file tests/unit/deployment_test.yaml -o junit`,
	Args: cobra.ExactArgs(1),
	RunE: runTestCmd,
}

func init() {
	testCmd.Flags().StringVar(&flagTestFile, "file", "", "Run a specific test file instead of discovering all")
	testCmd.Flags().StringVar(&flagTestTestsDir, "tests", "", "Directory to scan recursively for *_test.yaml (overrides test.testsDir; default: <chart>/tests)")
	testCmd.Flags().StringVar(&flagTestSnapshotDir, "snapshot-dir", "", "Directory for snapshot files (default: <chart>/tests/snapshots)")
	testCmd.Flags().BoolVar(&flagTestPassOnWarning, "pass-on-warning", false, "Exit 0 on run warnings such as no tests executed (default: exit 5)")
	rootCmd.AddCommand(testCmd)
}

func runTestCmd(cmd *cobra.Command, args []string) error {
	chartPath := args[0]

	slog.Debug("invoked", "command", "test", "chart", chartPath, "parallelism", flagParallelism)

	cfg, err := config.Load(chartPath)
	if err != nil {
		exitErr(3, "loading config: %v", err)
	}
	slog.Debug("loaded config", "release", cfg.Defaults.Release.Name, "namespace", cfg.Defaults.Release.Namespace)

	testsDir := resolveTestsDir(flagTestTestsDir, cfg.Test.TestsDir)
	var files []string
	if flagTestFile != "" {
		files = []string{flagTestFile}
	} else {
		files, err = runner.DiscoverTestFiles(chartPath, testsDir)
		if err != nil {
			exitErr(2, "discovering test files: %v", err)
		}
	}
	slog.Debug("discovered test files", "count", len(files), "testsDir", testsDir)

	// Warnings are non-fatal conditions that still shouldn't read as a green
	// "0 tests" pass in CI (a typo'd path, an empty tests dir, or files with no
	// tests). They fail the run (exit 5) by default; --pass-on-warning opts out.
	var warnings []string

	if len(files) == 0 {
		warnings = append(warnings, fmt.Sprintf("no unit test files found under %s", displayTestsRoot(chartPath, testsDir)))
	} else {
		opts := runner.Options{
			ChartPath:   chartPath,
			TestFiles:   files,
			Parallelism: flagParallelism,
			Cfg:         cfg,
			SnapshotDir: flagTestSnapshotDir,
		}

		results, err := runner.Run(opts)
		if err != nil {
			exitErr(2, "%v", err)
		}

		rep := selectReporter(flagOutput, os.Stdout, cienv.Detect())
		if err := rep.Report(results); err != nil {
			exitErr(2, "reporting: %v", err)
		}

		if runner.AnyFailed(results) {
			os.Exit(1)
		}
		if countTestCases(results) == 0 {
			warnings = append(warnings, "test files were discovered but contained no tests")
		}
	}

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "vigie: warning: %s\n", w)
	}
	if len(warnings) > 0 && !flagTestPassOnWarning {
		os.Exit(exitWarnings)
	}
	return nil
}

// countTestCases totals the executed test cases across all suites.
func countTestCases(results []runner.SuiteResult) int {
	total := 0
	for _, sr := range results {
		total += len(sr.Results)
	}
	return total
}

// resolveTestsDir picks the CLI --tests flag when set, else the config value.
func resolveTestsDir(flagValue, configValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return configValue
}

// displayTestsRoot returns the discovery root for user-facing messages.
func displayTestsRoot(chartPath, testsDir string) string {
	if testsDir == "" {
		return filepath.Join(chartPath, "tests")
	}
	if filepath.IsAbs(testsDir) {
		return testsDir
	}
	return filepath.Join(chartPath, testsDir)
}
