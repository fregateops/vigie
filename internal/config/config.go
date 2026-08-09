package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const filename = ".vigie.yaml"

type Config struct {
	Defaults  Defaults        `yaml:"defaults"`
	Lint      LintConfig      `yaml:"lint"`
	Validate  ValidateConfig  `yaml:"validate"`
	Test      TestConfig      `yaml:"test"`
	TestApply TestApplyConfig `yaml:"testApply"`
	Run       RunConfig       `yaml:"run"`
}

// LintConfig controls which rule sets run and what to ignore.
type LintConfig struct {
	// RuleSets is an allowlist of rule sets to run. Empty means "all defaults".
	RuleSets []string `yaml:"ruleSets"`
	// DisableRules is a denylist of individual rule IDs that must not run, even
	// if their rule set is enabled. Distinct from Ignore, which filters
	// findings post-execution by path.
	DisableRules []string     `yaml:"disableRules"`
	KubeVersions []string     `yaml:"kubeVersions"`
	Ignore       []IgnoreRule `yaml:"ignore"`
}

// IsRuleDisabled reports whether the given rule ID is in DisableRules.
func (c *LintConfig) IsRuleDisabled(id string) bool {
	for _, r := range c.DisableRules {
		if r == id {
			return true
		}
	}
	return false
}

// IgnoreRule suppresses a specific rule, optionally scoped to file paths.
type IgnoreRule struct {
	Rule  string   `yaml:"rule"`
	Paths []string `yaml:"paths"`
}

// EnabledRuleSets returns the configured rule sets, or all defaults if none specified.
func (c *LintConfig) EnabledRuleSets() map[string]bool {
	defaults := []string{"helm-v3-lint", "chart-yaml", "template-best-practices", "deprecation"}
	sets := c.RuleSets
	if len(sets) == 0 {
		sets = defaults
	}
	m := make(map[string]bool, len(sets))
	for _, s := range sets {
		m[s] = true
	}
	return m
}

// ValidateConfig controls `vigie validate` (chart-level: render chart with
// values.yaml + each overlay, then run kubeconform against the rendered docs).
type ValidateConfig struct {
	// ValuesFiles lists value overlays to validate, layered on the chart's
	// `values.yaml` (helm `-f overlay.yaml` semantics). Each entry produces one
	// independent render+kubeconform pass. Empty means "just the baseline
	// render against values.yaml".
	ValuesFiles []string `yaml:"valuesFiles"`
	// KubeVersions lists Kubernetes versions to validate against. Each
	// (overlay × kubeVersion) pair runs as a separate scenario.
	KubeVersions []string `yaml:"kubeVersions"`
	// Set holds --set style key=value overrides (helm strvals semantics). Applied
	// as the base layer; values files take higher priority.
	Set []string `yaml:"set"`
	// SetJSON holds --set-json style key=jsonValue overrides.
	SetJSON []string `yaml:"setJson"`
	// SetLiteral holds --set-literal style key=literalString overrides (no type coercion).
	SetLiteral []string `yaml:"setLiteral"`
	// Ignore suppresses specific schema violations.
	Ignore []ValidateIgnoreRule `yaml:"ignore"`
}

// ValidateIgnoreRule suppresses a kubeconform finding by kind, name, and
// optional regex over the violation message.
type ValidateIgnoreRule struct {
	Kind         string `yaml:"kind"`
	Name         string `yaml:"name"`
	MessageRegex string `yaml:"messageRegex"`
}

// TestConfig controls `vigie test` (template-tier).
type TestConfig struct {
	// SkipSchema disables the per-test kubeconform pass when true.
	SkipSchema bool `yaml:"skipSchema"`
	// KubeVersions used by the per-test kubeconform pass. Has no effect when
	// SkipSchema is true.
	KubeVersions []string `yaml:"kubeVersions"`
	// TestsDir overrides the discovery root for `vigie test`. Relative paths
	// resolve against the chart directory. Empty falls back to `<chart>/tests`.
	// The directory is scanned recursively for `*_test.yaml`.
	TestsDir string `yaml:"testsDir"`
}

// TestApplyConfig configures `vigie test-apply`. The cluster backend is
// selected via Cluster.Type — envtest (default), simulated, kind, k3d, or
// kubeconfig. See DESIGN.md §3 for the backend/tier mapping.
type TestApplyConfig struct {
	// Cluster pins the backend `test-apply` runs against.
	Cluster ClusterConfig `yaml:"cluster"`
	// TestsDir overrides the discovery root for `vigie test-apply`.
	// Relative paths resolve against the chart directory. Empty falls back
	// to `<chart>/tests`. The directory is scanned recursively for
	// `*_test.yaml`.
	TestsDir string `yaml:"testsDir"`
}

// RunConfig controls `vigie run` — the orchestrated command that chains
// lint → validate → test → test-apply. Lint, validate, and test always run;
// the apply tier is opt-in via ApplyTiers because real clusters are slow.
// Direct subcommand invocation (`vigie test-apply`) ignores this list —
// the user already opted in by typing the subcommand.
type RunConfig struct {
	// ApplyTiers selects which `test-apply` cluster backends `vigie run`
	// invokes. Empty/omitted means "no apply tiers — just lint+validate+test".
	// Valid values: any cluster backend type (envtest, simulated, kind, k3d,
	// kubeconfig).
	ApplyTiers []string `yaml:"applyTiers"`
}

// ClusterConfig selects the cluster backend for `test-apply`. Mirrors
// internal/cluster.Config but lives in config so charts can pin a backend in
// `.vigie.yaml` without depending on the cluster package.
type ClusterConfig struct {
	// Type selects the implementation: envtest | simulated | kind | k3d | kubeconfig.
	// Empty defaults to "envtest" — fast and dependency-free.
	Type string `yaml:"type"`
	// KubeVersion is the target Kubernetes server version. envtest uses it to
	// pin the binary asset version; node-backed backends (kind, k3d) use it to
	// pin the node image.
	KubeVersion string `yaml:"kubeVersion"`
	// Kubeconfig is the path to a kubeconfig file used when Type is
	// "kubeconfig". Mirrors `helm --kubeconfig`.
	Kubeconfig string `yaml:"kubeconfig"`
}

type Defaults struct {
	Release ReleaseDefaults `yaml:"release"`
}

type ReleaseDefaults struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

func DefaultConfig() *Config {
	return &Config{
		Defaults: Defaults{
			Release: ReleaseDefaults{
				Name:      "release-name",
				Namespace: "default",
			},
		},
		TestApply: TestApplyConfig{
			Cluster: ClusterConfig{Type: "envtest"},
		},
	}
}

// Load reads .vigie.yaml from chartDir. Returns defaults if the file is absent.
func Load(chartDir string) (*Config, error) {
	cfg := DefaultConfig()
	path := filepath.Join(chartDir, filename)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	// An empty file is valid and keeps the defaults.
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}
	// Strict decoding so a misspelled or misplaced key fails loudly instead of
	// being silently ignored (e.g. `ruleSet:` for `ruleSets:`).
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := validateConfigKubeVersions(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// validateConfigKubeVersions vets every Kubernetes version field that feeds a
// binary download (kubeconform schemas, envtest/kcm/scheduler) so a truncated
// "1.30" surfaces at config-load time instead of as a 404 from dl.k8s.io.
// lint.kubeVersions is intentionally skipped — it accepts MAJOR.MINOR and only
// drives helm template's `.Capabilities.KubeVersion`, no binary download.
func validateConfigKubeVersions(cfg *Config) error {
	if err := validateKubeVersions("validate.kubeVersions", cfg.Validate.KubeVersions); err != nil {
		return err
	}
	if err := validateKubeVersions("test.kubeVersions", cfg.Test.KubeVersions); err != nil {
		return err
	}
	if err := ValidateKubeVersion("testApply.cluster.kubeVersion", cfg.TestApply.Cluster.KubeVersion); err != nil {
		return err
	}
	return nil
}
