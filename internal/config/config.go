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
	Defaults Defaults       `yaml:"defaults"`
	Lint     LintConfig     `yaml:"lint"`
	Validate ValidateConfig `yaml:"validate"`
	Test     TestConfig     `yaml:"test"`
	Run      RunConfig      `yaml:"run"`
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

// TestConfig controls `vigie test` — both the template tier (render + assert
// in-process) and the cluster tiers reached with `--cluster <backend>`.
type TestConfig struct {
	// SkipSchema disables the per-test kubeconform pass when true.
	SkipSchema bool `yaml:"skipSchema"`
	// KubeVersions used by the per-test kubeconform pass. Has no effect when
	// SkipSchema is true.
	KubeVersions []string `yaml:"kubeVersions"`
	// TestsDir overrides the discovery root for `vigie test`, in every tier.
	// Relative paths resolve against the chart directory. Empty falls back to
	// `<chart>/tests`. The directory is scanned recursively for `*_test.yaml`.
	TestsDir string `yaml:"testsDir"`
	// Cluster holds the per-backend settings for the cluster tiers.
	Cluster ClusterConfig `yaml:"cluster"`
}

// RunConfig controls `vigie run` — the orchestrated command that chains
// lint → validate → test. Lint, validate, and the template tier always run;
// cluster (apply) tiers are opt-in via ApplyTiers because real clusters are
// slow. A direct `vigie test --cluster <type>` invocation ignores this list —
// the user already opted in by passing --cluster.
type RunConfig struct {
	// ApplyTiers selects which cluster backends `vigie run` exercises via the
	// apply tier. Empty/omitted means "no apply tiers — just lint+validate+test".
	// Valid values: any cluster backend type (envtest, simulated, kind, k3d,
	// kubeconfig).
	ApplyTiers []string `yaml:"applyTiers"`
}

// ClusterConfig groups the per-backend settings for the cluster tiers of
// `vigie test`. It does not select a backend — `--cluster <backend>` does, and
// only the matching sub-block is read for a given run. Charts can therefore
// pin every backend's settings once and switch tiers from the CLI.
type ClusterConfig struct {
	// Envtest configures the in-process apiserver backend (`--cluster envtest`).
	Envtest EnvtestConfig `yaml:"envtest"`
	// Kind configures the kind backend (`--cluster kind`).
	Kind NodeBackendConfig `yaml:"kind"`
	// K3d configures the k3d backend (`--cluster k3d`).
	K3d NodeBackendConfig `yaml:"k3d"`
	// Kubeconfig configures the external-cluster backend (`--cluster kubeconfig`).
	Kubeconfig KubeconfigBackendConfig `yaml:"kubeconfig"`
}

// EnvtestConfig configures the envtest backend, which runs a real
// kube-apiserver and etcd in-process with no controllers.
type EnvtestConfig struct {
	// KubeVersion pins the envtest binary asset version. Empty uses the
	// built-in default. Overridden by `--kube-version`.
	KubeVersion string `yaml:"kubeVersion"`
}

// NodeBackendConfig configures a node-backed backend (kind or k3d), each of
// which provisions a throwaway cluster by driving its external CLI.
type NodeBackendConfig struct {
	// KubeVersion pins the node image version. Empty uses the CLI's default.
	// Overridden by `--kube-version`.
	KubeVersion string `yaml:"kubeVersion"`
	// Binary is the path to the backend's CLI. Empty resolves it from PATH,
	// then the vigie cache, then an optional download. Overridden by
	// `--kind-binary` / `--k3d-binary`.
	Binary string `yaml:"binary"`
	// ExtraArgs are additional flags passed verbatim to the provisioning CLI.
	// Example: ["--config", "kind-3node.yaml"] for kind, or
	// ["-v", "/host:/node"] for k3d.
	ExtraArgs []string `yaml:"extraArgs"`
}

// KubeconfigBackendConfig configures the kubeconfig backend, which runs against
// a cluster the user already operates.
type KubeconfigBackendConfig struct {
	// Path is the kubeconfig file to reach the cluster with. Mirrors
	// `helm --kubeconfig`. Overridden by `--kubeconfig`.
	Path string `yaml:"path"`
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
	if err := checkRetiredKeys(data); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
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

// retiredKeys maps top-level keys removed by the v2 config model to the
// migration hint shown when a stale `.vigie.yaml` still carries them. Strict
// decoding would reject them anyway, but with a bare "field not found" that
// says nothing about where the settings moved.
var retiredKeys = map[string]string{
	"testApply": "move `testApply.cluster` settings under `test.cluster.<backend>` " +
		"(envtest, kind, k3d, kubeconfig) and `testApply.testsDir` to `test.testsDir`",
}

// checkRetiredKeys reports a v1 config key with its v2 home, pointing at the
// line the key sits on. A malformed or non-mapping document is left to the
// strict decode, which reports the parse error.
func checkRetiredKeys(data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	// A mapping's Content alternates key, value — only the keys interest us.
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i]
		if hint, ok := retiredKeys[key.Value]; ok {
			return fmt.Errorf("line %d: `%s:` was removed in the v2 config model: %s", key.Line, key.Value, hint)
		}
	}
	return nil
}

// validateConfigKubeVersions vets every Kubernetes version field that feeds a
// binary download (kubeconform schemas, envtest/kcm/scheduler) or a node image
// so a truncated "1.30" surfaces at config-load time instead of as a 404 from
// dl.k8s.io. lint.kubeVersions is intentionally skipped — it accepts
// MAJOR.MINOR and only drives helm template's `.Capabilities.KubeVersion`, no
// binary download.
func validateConfigKubeVersions(cfg *Config) error {
	if err := validateKubeVersions("validate.kubeVersions", cfg.Validate.KubeVersions); err != nil {
		return err
	}
	if err := validateKubeVersions("test.kubeVersions", cfg.Test.KubeVersions); err != nil {
		return err
	}
	clusterVersions := []struct {
		field   string
		version string
	}{
		{"test.cluster.envtest.kubeVersion", cfg.Test.Cluster.Envtest.KubeVersion},
		{"test.cluster.kind.kubeVersion", cfg.Test.Cluster.Kind.KubeVersion},
		{"test.cluster.k3d.kubeVersion", cfg.Test.Cluster.K3d.KubeVersion},
	}
	for _, cv := range clusterVersions {
		if err := ValidateKubeVersion(cv.field, cv.version); err != nil {
			return err
		}
	}
	return nil
}
