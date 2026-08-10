package runner

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/render"
	"helm.sh/helm/v3/pkg/strvals"
)

// ValidateOptions controls chart-level `vigie validate`.
type ValidateOptions struct {
	ChartPath string
	// Overlays lists value-file paths to layer on top of the chart's values.yaml
	// (helm `-f` semantics). Empty/nil means "one baseline render against
	// values.yaml alone".
	Overlays []string
	// KubeVersions lists Kubernetes versions to validate against. Empty defaults
	// to [config.DefaultKubeVersion]. Each (overlay × kubeVersion) pair runs as a
	// separate scenario.
	KubeVersions []string
	// Set holds --set style key=value pairs (helm strvals semantics). Applied as
	// the base layer; overlays (values files) take higher priority.
	Set []string
	// SetJSON holds --set-json style key=jsonValue pairs.
	SetJSON []string
	// SetLiteral holds --set-literal style key=literalString pairs (no type coercion).
	SetLiteral  []string
	Parallelism int
	Cfg         *config.Config
	// Ignore is the chart-level config's kubeconform ignore list, applied to
	// every scenario.
	Ignore []config.ValidateIgnoreRule
}

// RunValidate runs the chart-level validate flow: for each (overlay,
// kubeVersion) scenario, render the chart and run kubeconform on the rendered
// docs. Returns one SuiteResult per overlay (or one for the baseline if no
// overlays), each containing one TestResult per kubeVersion.
func RunValidate(opts ValidateOptions) ([]SuiteResult, error) {
	kubeVersions := opts.KubeVersions
	if len(kubeVersions) == 0 {
		kubeVersions = []string{config.DefaultKubeVersion}
	}

	// One SuiteResult per overlay (or one for "baseline" if no overlays).
	groups := opts.Overlays
	if len(groups) == 0 {
		groups = []string{""} // empty string = baseline render against values.yaml
	}

	// Pre-build one SchemaValidator per kubeVersion; reused across all overlays
	// so the in-memory schema cache amortises across scenarios.
	validators := make(map[string]*render.SchemaValidator, len(kubeVersions))
	for _, kv := range kubeVersions {
		sv, err := render.NewSchemaValidator(kv)
		if err != nil {
			return nil, fmt.Errorf("creating schema validator for %s: %w", kv, err)
		}
		validators[kv] = sv
	}

	return runParallel(groups, opts.Parallelism, func(_ int, overlayPath string) (SuiteResult, error) {
		return runValidateOverlay(opts, overlayPath, kubeVersions, validators)
	})
}

func runValidateOverlay(opts ValidateOptions, overlayPath string, kubeVersions []string, validators map[string]*render.SchemaValidator) (SuiteResult, error) {
	suiteName := "validate: values.yaml"
	if overlayPath != "" {
		suiteName = fmt.Sprintf("validate: values.yaml + %s", filepath.Base(overlayPath))
	}
	sr := SuiteResult{Suite: suiteName, File: overlayPath}
	suiteStart := time.Now()

	var overlay map[string]any
	if overlayPath != "" {
		loaded, err := render.LoadValuesFile(overlayPath)
		if err != nil {
			sr.Results = append(sr.Results, TestResult{
				SuiteName: suiteName,
				TestName:  fmt.Sprintf("load %s", overlayPath),
				Pass:      false,
				Failures:  []string{fmt.Sprintf("        → %v", err)},
				Duration:  time.Since(suiteStart),
			})
			sr.Duration = time.Since(suiteStart)
			return sr, nil
		}
		overlay = loaded
	}

	baseValues, err := buildBaseValues(opts)
	if err != nil {
		return SuiteResult{Suite: suiteName, File: overlayPath, Duration: time.Since(suiteStart)}, err
	}

	for _, kv := range kubeVersions {
		tr := TestResult{
			SuiteName: suiteName,
			TestName:  fmt.Sprintf("k8s %s", kv),
		}
		start := time.Now()

		req := render.Request{
			ChartPath:     opts.ChartPath,
			Values:        baseValues,
			OverlayValues: overlay,
			ReleaseName:   opts.Cfg.Defaults.Release.Name,
			Namespace:     opts.Cfg.Defaults.Release.Namespace,
			KubeVersion:   kv,
		}
		clog.Trace("validate render",
			"chart", opts.ChartPath, "overlay", overlayPath, "kubeVersion", kv)
		result, err := render.Render(req)
		if err != nil {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        render: %v", err))
			tr.Duration = time.Since(start)
			sr.Results = append(sr.Results, tr)
			continue
		}

		sv := validators[kv]
		schemaErrs, err := sv.Validate(result.Docs)
		if err != nil {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        schema validation error: %v", err))
		}
		for _, se := range schemaErrs {
			if shouldIgnoreSchemaError(se, opts.Ignore) {
				continue
			}
			tr.Failures = append(tr.Failures, fmt.Sprintf("        schema: %s/%s: %s", se.Kind, se.Name, se.Message))
		}

		tr.Pass = len(tr.Failures) == 0
		tr.Duration = time.Since(start)
		sr.Results = append(sr.Results, tr)
		slog.Debug("validate scenario finished",
			"overlay", overlayPath, "kubeVersion", kv,
			"pass", tr.Pass, "duration", tr.Duration)
	}

	sr.Duration = time.Since(suiteStart)
	return sr, nil
}

func buildBaseValues(opts ValidateOptions) (map[string]any, error) {
	base := map[string]any{}
	for _, s := range opts.Set {
		if err := strvals.ParseInto(s, base); err != nil {
			return nil, fmt.Errorf("--set: %w", err)
		}
	}
	for _, s := range opts.SetJSON {
		if err := strvals.ParseJSON(s, base); err != nil {
			return nil, fmt.Errorf("--set-json: %w", err)
		}
	}
	for _, s := range opts.SetLiteral {
		if err := strvals.ParseLiteralInto(s, base); err != nil {
			return nil, fmt.Errorf("--set-literal: %w", err)
		}
	}
	return base, nil
}

func shouldIgnoreSchemaError(se render.SchemaError, rules []config.ValidateIgnoreRule) bool {
	for _, r := range rules {
		if r.Kind != "" && r.Kind != se.Kind {
			continue
		}
		if r.Name != "" && r.Name != se.Name {
			continue
		}
		if r.MessageRegex != "" {
			ok, err := regexp.MatchString(r.MessageRegex, se.Message)
			if err != nil || !ok {
				continue
			}
		}
		return true
	}
	return false
}
