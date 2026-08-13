package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/runner"
	"github.com/spf13/cobra"
)

var (
	flagValidateValues       []string
	flagValidateKubeVersions []string
	flagValidateSet          []string
	flagValidateSetJSON      []string
	flagValidateSetLiteral   []string
)

var validateCmd = &cobra.Command{
	Use:   "validate [chart]",
	Short: "Render the chart against each values overlay and validate output with kubeconform",
	Long: `validate is a chart-level smoke check. It renders the chart with the
implicit baseline (values.yaml) plus any value overlays you supply, then runs
kubeconform on the rendered manifests for each Kubernetes version. Overlays use
helm '-f' semantics — each overlay is layered on top of values.yaml and runs as
an independent scenario.

No test files are consumed.`,
	Example: `  # Validate the baseline render (values.yaml) against the default kube version
  vigie validate ./mychart

  # Layer two overlays; each runs as an independent scenario
  vigie validate ./mychart --values values-prod.yaml --values values-eu.yaml

  # Validate across multiple Kubernetes versions with a --set override
  vigie validate ./mychart --kube-version 1.33.0,1.36.1 --set replicaCount=3

  # Emit SARIF for a CI code-scanning upload
  vigie validate ./mychart -o sarif`,
	Args: cobra.MaximumNArgs(1),
	RunE: runValidateCmd,
}

func init() {
	validateCmd.Flags().IntVarP(&flagParallelism, "parallelism", "p", runtime.NumCPU(), "Number of parallel scenarios")
	validateCmd.Flags().StringSliceVar(&flagValidateValues, "values", nil,
		"Value overlay files (helm `-f` semantics). Repeat or comma-separate; each overlay produces an independent render.")
	validateCmd.Flags().StringSliceVar(&flagValidateKubeVersions, "kube-version", nil,
		"Kubernetes versions to validate against. Repeat or comma-separate. Default: 1.36.1")
	validateCmd.Flags().StringArrayVar(&flagValidateSet, "set", nil,
		"set values on the command line (can specify multiple, Helm --set semantics: key=val,key2=val2)")
	validateCmd.Flags().StringArrayVar(&flagValidateSetJSON, "set-json", nil,
		"set JSON values on the command line (key=jsonVal)")
	validateCmd.Flags().StringArrayVar(&flagValidateSetLiteral, "set-literal", nil,
		"set a literal string value on the command line (key=literalVal)")
	rootCmd.AddCommand(validateCmd)
}

func runValidateCmd(cmd *cobra.Command, args []string) error {
	chartPath := argOrCwd(args)

	for idx, v := range flagValidateKubeVersions {
		if err := config.ValidateKubeVersion(fmt.Sprintf("--kube-version[%d]", idx), v); err != nil {
			exitErr(3, "%v", err)
		}
	}

	cfg, err := config.Load(chartPath)
	if err != nil {
		exitErr(3, "loading config: %v", err)
	}

	overlays := flagValidateValues
	if len(overlays) == 0 {
		overlays = cfg.Validate.ValuesFiles
	}
	overlays = resolveOverlayPaths(chartPath, overlays)

	kubeVersions := flagValidateKubeVersions
	if len(kubeVersions) == 0 {
		kubeVersions = cfg.Validate.KubeVersions
	}

	set := flagValidateSet
	if len(set) == 0 {
		set = cfg.Validate.Set
	}
	setJSON := flagValidateSetJSON
	if len(setJSON) == 0 {
		setJSON = cfg.Validate.SetJSON
	}
	setLiteral := flagValidateSetLiteral
	if len(setLiteral) == 0 {
		setLiteral = cfg.Validate.SetLiteral
	}

	slog.Debug("invoked",
		"command", "validate", "chart", chartPath,
		"overlays", overlays, "kubeVersions", kubeVersions)

	opts := runner.ValidateOptions{
		ChartPath:    chartPath,
		Overlays:     overlays,
		KubeVersions: kubeVersions,
		Set:          set,
		SetJSON:      setJSON,
		SetLiteral:   setLiteral,
		Parallelism:  flagParallelism,
		Cfg:          cfg,
		Ignore:       cfg.Validate.Ignore,
	}

	results, err := runner.RunValidate(opts)
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
	return nil
}

// resolveOverlayPaths makes overlay paths absolute or relative-to-chart.
// Absolute paths and paths with explicit "./" / "../" are kept as-is. Bare
// names like "values-prod.yaml" are resolved relative to chartPath.
func resolveOverlayPaths(chartPath string, overlays []string) []string {
	if len(overlays) == 0 {
		return nil
	}
	out := make([]string, 0, len(overlays))
	for _, p := range overlays {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
			out = append(out, p)
			continue
		}
		out = append(out, filepath.Join(chartPath, p))
	}
	return out
}
