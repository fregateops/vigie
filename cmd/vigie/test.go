package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/doctor"
	"github.com/fregateops/vigie/internal/runner"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	flagTestFile            string
	flagTestTestsDir        string
	flagTestSnapshotDir     string
	flagTestPassOnWarning   bool
	flagTestSchema          bool
	flagTestKubeVersions    []string
	flagTestCluster         string
	flagTestKubeconfig      string
	flagTestFailFast        bool
	flagTestKeepCluster     bool
	flagTestMatch           string
	flagTestUpdateSnapshots bool
	flagTestKindBinary      string
	flagTestK3dBinary       string
	flagTestDownloadTools   bool
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
	Use:   "test [chart]",
	Short: "Render templates per test and run user assertions (optionally against a cluster)",
	Long: "Run a chart's tests. By default (--cluster none) templates are rendered in-process\n" +
		"and assertions run against the rendered manifests. Pass --cluster to install each\n" +
		"test's chart into a real control plane and assert against the live objects:\n\n" +
		"  envtest     real kube-apiserver + etcd, no controllers (fast, dependency-free)\n" +
		"  kubeconfig  a cluster you already run, reached via an external kubeconfig\n" +
		"  kind, k3d   a throwaway node-backed cluster provisioned via the kind/k3d CLI\n\n" +
		"simulated arrives in a later release.",
	Example: `  # Template tier: render every tests/*_test.yaml under the chart
  vigie test ./mychart

  # Apply tier: install each test against an in-process apiserver (envtest)
  vigie test ./mychart --cluster envtest

  # Integration tier: run against a cluster you already have
  vigie test ./mychart --cluster kubeconfig --kubeconfig ~/.kube/config

  # E2e tier: provision a throwaway kind cluster (kind resolved from PATH)
  vigie test ./mychart --cluster kind

  # Run a single test file and emit JUnit for CI
  vigie test ./mychart --file tests/unit/deployment_test.yaml -o junit`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTestCmd,
}

func init() {
	testCmd.Flags().IntVarP(&flagParallelism, "parallelism", "p", runtime.NumCPU(), "Number of parallel test files")
	testCmd.Flags().StringVar(&flagTestFile, "file", "", "Run a specific test file instead of discovering all")
	testCmd.Flags().StringVar(&flagTestTestsDir, "tests", "", "Directory to scan recursively for *_test.yaml (overrides test.testsDir; default: <chart>/tests)")
	testCmd.Flags().StringVar(&flagTestSnapshotDir, "snapshot-dir", "", "Directory for snapshot files (default: <chart>/tests/snapshots)")
	testCmd.Flags().BoolVar(&flagTestPassOnWarning, "pass-on-warning", false, "Exit 0 on run warnings such as no tests executed (default: exit 5)")
	testCmd.Flags().BoolVar(&flagTestSchema, "schema", true, "Run the per-test kubeconform pass (template tier)")
	testCmd.Flags().StringSliceVar(&flagTestKubeVersions, "kube-version", nil, "Kubernetes version(s), repeatable: template tier matrixes kubeconform over all; cluster tier uses the first (default: 1.36.1)")
	testCmd.Flags().StringVar(&flagTestCluster, "cluster", clusterNone, "Cluster backend for the apply tier: none|envtest|simulated|kind|k3d|kubeconfig (default none = template tier)")
	testCmd.Flags().StringVar(&flagTestKubeconfig, "kubeconfig", "", "Path to kubeconfig for --cluster kubeconfig (overrides testApply.cluster.kubeconfig)")
	testCmd.Flags().BoolVar(&flagTestFailFast, "fail-fast", false, "Cancel queued tests after the first failure")
	testCmd.Flags().BoolVar(&flagTestKeepCluster, "keep-cluster", false, "Keep the cluster running after the suite for debugging (node-backed backends only)")
	testCmd.Flags().StringVar(&flagTestMatch, "match", "", "Run only tests whose display name matches this regex")
	testCmd.Flags().BoolVarP(&flagTestUpdateSnapshots, "update-snapshots", "u", false, "Update snapshots on mismatch instead of failing")
	testCmd.Flags().StringVar(&flagTestKindBinary, "kind-binary", "", "Path to the kind CLI (default: resolve from PATH, then the vigie cache, then download)")
	testCmd.Flags().StringVar(&flagTestK3dBinary, "k3d-binary", "", "Path to the k3d CLI (default: resolve from PATH, then the vigie cache, then download)")
	testCmd.Flags().BoolVar(&flagTestDownloadTools, "download-tools", false, "Download a missing kind/k3d CLI without prompting (for CI/automation; also VIGIE_AUTO_DOWNLOAD)")
	rootCmd.AddCommand(testCmd)
}

func runTestCmd(cmd *cobra.Command, args []string) error {
	chartPath := argOrCwd(args)

	slog.Debug("invoked", "command", "test", "chart", chartPath,
		"cluster", flagTestCluster, "parallelism", flagParallelism)

	for idx, ver := range flagTestKubeVersions {
		if err := config.ValidateKubeVersion(fmt.Sprintf("--kube-version[%d]", idx), ver); err != nil {
			exitErr(3, "%v", err)
		}
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
		// Schema validation is on by default; --schema=false or test.skipSchema disables it.
		skipSchema := !flagTestSchema || cfg.Test.SkipSchema
		// The template tier matrixes kubeconform over every requested version;
		// the flag wins, else the chart's test.kubeVersions, else the default.
		kubeVersions := flagTestKubeVersions
		if len(kubeVersions) == 0 {
			kubeVersions = cfg.Test.KubeVersions
		}
		return runner.Run(runner.Options{
			ChartPath:       chartPath,
			TestFiles:       files,
			Parallelism:     flagParallelism,
			Cfg:             cfg,
			SnapshotDir:     flagTestSnapshotDir,
			SnapshotUpdate:  flagTestUpdateSnapshots,
			ValidateSchemas: !skipSchema,
			KubeVersions:    kubeVersions,
			Match:           flagTestMatch,
			FailFast:        flagTestFailFast,
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
		ChartPath:      chartPath,
		TestFiles:      files,
		Parallelism:    flagParallelism,
		Cfg:            cfg,
		Backend:        backend,
		BackendType:    clusterCfg.Type,
		SnapshotDir:    flagTestSnapshotDir,
		SnapshotUpdate: flagTestUpdateSnapshots,
		Match:          flagTestMatch,
		FailFast:       flagTestFailFast,
		KeepCluster:    flagTestKeepCluster,
	})
}

// resolveClusterConfig builds the cluster.Config from the --cluster flag,
// layering --kube-version / --kubeconfig over the chart's testApply.cluster
// settings. The backend type comes from the flag (already known to be a real
// backend, not "none"). A cluster pins a single Kubernetes version, so when
// --kube-version lists several the first wins and the rest are warned about.
func resolveClusterConfig(cfg *config.Config) cluster.Config {
	configured := cfg.TestApply.Cluster
	resolved := cluster.Config{
		Type:        flagTestCluster,
		KubeVersion: configured.KubeVersion,
		Kubeconfig:  configured.Kubeconfig,
	}
	if len(flagTestKubeVersions) > 0 {
		resolved.KubeVersion = flagTestKubeVersions[0]
		if len(flagTestKubeVersions) > 1 {
			fmt.Fprintf(os.Stderr, "vigie: warning: --cluster %s pins one Kubernetes version; using %s and ignoring %d more\n",
				flagTestCluster, flagTestKubeVersions[0], len(flagTestKubeVersions)-1)
		}
	}
	if flagTestKubeconfig != "" {
		resolved.Kubeconfig = flagTestKubeconfig
	}
	// Node-backed backends (kind, k3d) resolve their CLI; carry the binary
	// overrides and the download policy. Other backends ignore these fields.
	resolved.KindBinary = flagTestKindBinary
	resolved.K3dBinary = flagTestK3dBinary
	resolved.ToolDownload, resolved.ConfirmDownload = toolDownloadPolicy()
	resolved.Progress = os.Stderr
	return resolved
}

// toolDownloadPolicy decides whether a missing kind/k3d CLI may be fetched.
// An explicit opt-in (--download-tools / VIGIE_AUTO_DOWNLOAD) downloads without
// prompting; an interactive terminal (not CI) prompts for confirmation;
// everything else (CI, piped stdin) never downloads and errors with guidance.
func toolDownloadPolicy() (doctor.DownloadPolicy, func(prompt string) bool) {
	if flagTestDownloadTools || os.Getenv("VIGIE_AUTO_DOWNLOAD") != "" {
		return doctor.DownloadAlways, nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) && cienv.Detect() == cienv.KindNone && os.Getenv("CI") == "" {
		return doctor.DownloadInteractive, confirmDownload
	}
	return doctor.DownloadNever, nil
}

// confirmDownload prompts on stderr and reads a yes/no answer from stdin.
func confirmDownload(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
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
