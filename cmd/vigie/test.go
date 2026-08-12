package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/runner"
	"github.com/spf13/cobra"
)

var (
	flagTestFile          string
	flagTestTestsDir      string
	flagTestSnapshotDir   string
	flagTestPassOnWarning bool
	flagTestNoSchema      bool
	flagTestKubeVersion   string
	flagTestCluster       string
	flagTestKubeconfig    string
	flagTestFailFast      bool
	flagTestKeepCluster   bool
)

// clusterNone is the default --cluster value: run the in-process template tier
// (render + assert + kubeconform), no cluster backend.
const clusterNone = "none"

// exitWarnings is the exit code for a run that produced only warnings (e.g. no
// tests executed) - distinct from setup (2)/user (3) errors so CI can tell
// "your chart has no tests" apart from "the tool broke". Mirrors pytest's
// dedicated "no tests collected" code.
const exitWarnings = 5

var testCmd = &cobra.Command{
	Use:   "test <chart>",
	Short: "Render templates per test and run user assertions (optionally against a cluster)",
	Long: "Run a chart's tests. By default (--cluster none) templates are rendered in-process\n" +
		"and assertions run against the rendered manifests. Pass --cluster to install each\n" +
		"test's chart into a real control plane and assert against the live objects:\n\n" +
		"  envtest  real kube-apiserver + etcd, no controllers (fast, dependency-free)\n\n" +
		"simulated, kind, k3d, and kubeconfig arrive in later releases.",
	Example: `  # Template tier: render every tests/*_test.yaml under the chart
  vigie test ./mychart

  # Apply tier: install each test against an in-process apiserver (envtest)
  vigie test ./mychart --cluster envtest

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
	testCmd.Flags().BoolVar(&flagTestNoSchema, "no-schema", false, "Skip the per-test kubeconform pass (template tier; on by default)")
	testCmd.Flags().StringVar(&flagTestKubeVersion, "kube-version", "", "Kubernetes version: kubeconform pass (template tier) or cluster backend version (default: 1.36.1)")
	testCmd.Flags().StringVar(&flagTestCluster, "cluster", clusterNone, "Cluster backend for the apply tier: none|envtest|simulated|kind|k3d|kubeconfig (default none = template tier)")
	testCmd.Flags().StringVar(&flagTestKubeconfig, "kubeconfig", "", "Path to kubeconfig for --cluster kubeconfig (overrides testApply.cluster.kubeconfig)")
	testCmd.Flags().BoolVar(&flagTestFailFast, "fail-fast", false, "Cancel queued tests after the first failure (cluster tiers)")
	testCmd.Flags().BoolVar(&flagTestKeepCluster, "keep-cluster", false, "Keep the cluster running after the suite for debugging (node-backed backends only)")
	rootCmd.AddCommand(testCmd)
}

func runTestCmd(cmd *cobra.Command, args []string) error {
	chartPath := args[0]

	slog.Debug("invoked", "command", "test", "chart", chartPath,
		"cluster", flagTestCluster, "parallelism", flagParallelism)

	if err := config.ValidateKubeVersion("--kube-version", flagTestKubeVersion); err != nil {
		exitErr(3, "%v", err)
	}

	cfg, err := config.Load(chartPath)
	if err != nil {
		exitErr(3, "loading config: %v", err)
	}
	slog.Debug("loaded config", "release", cfg.Defaults.Release.Name, "namespace", cfg.Defaults.Release.Namespace)

	testsDir := resolveTestsDir(flagTestTestsDir, cfg.Test.TestsDir)
	files, err := discoverTests(chartPath, testsDir)
	if err != nil {
		exitErr(2, "discovering test files: %v", err)
	}
	slog.Debug("discovered test files", "count", len(files), "testsDir", testsDir)

	// Warnings are non-fatal conditions that still shouldn't read as a green
	// "0 tests" pass in CI (a typo'd path, an empty tests dir, or files with no
	// tests). They fail the run (exit 5) by default; --pass-on-warning opts out.
	var warnings []string

	if len(files) == 0 {
		warnings = append(warnings, fmt.Sprintf("no test files found under %s", displayTestsRoot(chartPath, testsDir)))
	} else {
		results, err := runTests(cmd.Context(), chartPath, cfg, files)
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

	emitWarnings(warnings)
	return nil
}

// discoverTests resolves the test-file list for the active tier. A single
// --file short-circuits discovery; otherwise the apply tier accepts both unit
// and integration suite shapes (DiscoverApplyTestFiles) while the template tier
// takes unit files only.
func discoverTests(chartPath, testsDir string) ([]string, error) {
	if flagTestFile != "" {
		return []string{flagTestFile}, nil
	}
	if flagTestCluster != clusterNone {
		return runner.DiscoverApplyTestFiles(chartPath, testsDir)
	}
	return runner.DiscoverTestFiles(chartPath, testsDir)
}

// runTests executes the discovered files under the active tier and returns the
// shared []runner.SuiteResult: the in-process template tier (--cluster none,
// render + assert + kubeconform) or the apply tier, which installs each test
// against the selected live cluster backend.
func runTests(ctx context.Context, chartPath string, cfg *config.Config, files []string) ([]runner.SuiteResult, error) {
	if flagTestCluster == clusterNone {
		// Schema validation is on by default; --no-schema or test.skipSchema disables it.
		skipSchema := flagTestNoSchema || cfg.Test.SkipSchema
		kubeVersion := flagTestKubeVersion
		if kubeVersion == "" && len(cfg.Test.KubeVersions) > 0 {
			kubeVersion = cfg.Test.KubeVersions[0]
		}
		return runner.Run(runner.Options{
			ChartPath:       chartPath,
			TestFiles:       files,
			Parallelism:     flagParallelism,
			Cfg:             cfg,
			SnapshotDir:     flagTestSnapshotDir,
			ValidateSchemas: !skipSchema,
			KubeVersion:     kubeVersion,
		})
	}

	clusterCfg := resolveClusterConfig(cfg)
	slog.Debug("cluster test", "backend", clusterCfg.Type,
		"kubeVersion", clusterCfg.KubeVersion, "kubeconfig", clusterCfg.Kubeconfig)
	backend, err := cluster.New(clusterCfg)
	if err != nil {
		exitErr(3, "configuring cluster backend: %v", err)
	}
	return runner.RunApply(ctx, runner.ApplyOptions{
		ChartPath:   chartPath,
		TestFiles:   files,
		Parallelism: flagParallelism,
		Cfg:         cfg,
		Backend:     backend,
		BackendType: clusterCfg.Type,
		SnapshotDir: flagTestSnapshotDir,
		FailFast:    flagTestFailFast,
		KeepCluster: flagTestKeepCluster,
	})
}

// resolveClusterConfig builds the cluster.Config from the --cluster flag,
// layering --kube-version / --kubeconfig over the chart's testApply.cluster
// settings. The backend type comes from the flag (already known to be a real
// backend, not "none").
func resolveClusterConfig(cfg *config.Config) cluster.Config {
	configured := cfg.TestApply.Cluster
	resolved := cluster.Config{
		Type:        flagTestCluster,
		KubeVersion: configured.KubeVersion,
		Kubeconfig:  configured.Kubeconfig,
	}
	if flagTestKubeVersion != "" {
		resolved.KubeVersion = flagTestKubeVersion
	}
	if flagTestKubeconfig != "" {
		resolved.Kubeconfig = flagTestKubeconfig
	}
	return resolved
}

// emitWarnings prints run warnings to stderr and exits with exitWarnings unless
// --pass-on-warning is set.
func emitWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "vigie: warning: %s\n", w)
	}
	if len(warnings) > 0 && !flagTestPassOnWarning {
		os.Exit(exitWarnings)
	}
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
