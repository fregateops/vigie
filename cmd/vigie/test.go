package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/report"
	"github.com/fregateops/vigie/internal/runner"
	"github.com/spf13/cobra"
)

var (
	flagTestFile     string
	flagTestTestsDir string
)

var testCmd = &cobra.Command{
	Use:   "test <chart>",
	Short: "Render templates per test and run user assertions",
	Args:  cobra.ExactArgs(1),
	RunE:  runTestCmd,
}

func init() {
	testCmd.Flags().StringVar(&flagTestFile, "file", "", "Run a specific test file instead of discovering all")
	testCmd.Flags().StringVar(&flagTestTestsDir, "tests", "", "Directory to scan recursively for *_test.yaml (overrides test.testsDir; default: <chart>/tests)")
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

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "vigie: no unit test files found under %s\n", displayTestsRoot(chartPath, testsDir))
		os.Exit(0)
	}

	opts := runner.Options{
		ChartPath:   chartPath,
		TestFiles:   files,
		Parallelism: flagParallelism,
		Cfg:         cfg,
	}

	results, err := runner.Run(opts)
	if err != nil {
		exitErr(2, "%v", err)
	}

	rep := &report.PrettyReporter{Out: os.Stdout, CI: cienv.Detect()}
	if err := rep.Report(results); err != nil {
		exitErr(2, "reporting: %v", err)
	}

	if runner.AnyFailed(results) {
		os.Exit(1)
	}
	return nil
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
