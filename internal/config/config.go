package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const filename = ".vigie.yaml"

// Config is the root of `.vigie.yaml`, the per-chart configuration file. Every
// key is optional; an absent or empty file uses the built-in defaults.
//
// The `json` tags mirror the `yaml` ones because tools/gen-schema reflects this
// type into the published JSON Schema, and invopop/jsonschema reads `json`.
// Every field is `omitempty` so the schema marks none of them required.
type Config struct {
	Defaults Defaults       `yaml:"defaults" json:"defaults,omitempty"`
	Lint     LintConfig     `yaml:"lint" json:"lint,omitempty"`
	Validate ValidateConfig `yaml:"validate" json:"validate,omitempty"`
	Test     TestConfig     `yaml:"test" json:"test,omitempty"`
	Run      RunConfig      `yaml:"run" json:"run,omitempty"`
}

// LintConfig controls which rule sets run and what to ignore.
type LintConfig struct {
	// RuleSets is an allowlist of rule sets to run. Empty means "all defaults".
	RuleSets []string `yaml:"ruleSets" json:"ruleSets,omitempty"`
	// DisableRules is a denylist of individual rule IDs that must not run, even
	// if their rule set is enabled. Distinct from Ignore, which filters
	// findings post-execution by path.
	DisableRules []string `yaml:"disableRules" json:"disableRules,omitempty"`
	// KubeVersions lists the Kubernetes versions to render against for
	// deprecation and version-aware rules. Each is a `MAJOR.MINOR` string.
	// Empty means "all supported versions".
	KubeVersions []string `yaml:"kubeVersions" json:"kubeVersions,omitempty"`
	// Ignore suppresses findings for a rule, optionally scoped to file paths.
	Ignore []IgnoreRule `yaml:"ignore" json:"ignore,omitempty"`
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
	// Rule is the namespaced rule ID to suppress, e.g.
	// `template-best-practices_missing-resource-limits`.
	Rule string `yaml:"rule" json:"rule,omitempty"`
	// Paths are glob patterns matched against the finding's source file path.
	// Empty suppresses the rule everywhere.
	Paths []string `yaml:"paths" json:"paths,omitempty"`
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
	ValuesFiles []string `yaml:"valuesFiles" json:"valuesFiles,omitempty"`
	// KubeVersions lists Kubernetes versions to validate against. Each
	// (overlay × kubeVersion) pair runs as a separate scenario.
	KubeVersions []string `yaml:"kubeVersions" json:"kubeVersions,omitempty"`
	// Set holds --set style key=value overrides (helm strvals semantics). Applied
	// as the base layer; values files take higher priority.
	Set []string `yaml:"set" json:"set,omitempty"`
	// SetJSON holds --set-json style key=jsonValue overrides.
	SetJSON []string `yaml:"setJson" json:"setJson,omitempty"`
	// SetLiteral holds --set-literal style key=literalString overrides (no type coercion).
	SetLiteral []string `yaml:"setLiteral" json:"setLiteral,omitempty"`
	// Ignore suppresses specific schema violations.
	Ignore []ValidateIgnoreRule `yaml:"ignore" json:"ignore,omitempty"`
}

// ValidateIgnoreRule suppresses a kubeconform finding by kind, name, and
// optional regex over the violation message.
type ValidateIgnoreRule struct {
	// Kind is the Kubernetes kind whose findings are suppressed, e.g. `Ingress`.
	Kind string `yaml:"kind" json:"kind,omitempty"`
	// Name is the object name whose findings are suppressed. Empty matches any.
	Name string `yaml:"name" json:"name,omitempty"`
	// MessageRegex further narrows the suppression to violation messages
	// matching this regular expression. Empty matches any.
	MessageRegex string `yaml:"messageRegex" json:"messageRegex,omitempty"`
}

// TestConfig controls `vigie test` — both the template tier (render + assert
// in-process) and the cluster tiers reached with `--cluster <backend>`.
type TestConfig struct {
	// SkipSchema disables the per-test kubeconform pass when true.
	SkipSchema bool `yaml:"skipSchema" json:"skipSchema,omitempty"`
	// KubeVersions used by the per-test kubeconform pass. Has no effect when
	// SkipSchema is true.
	KubeVersions []string `yaml:"kubeVersions" json:"kubeVersions,omitempty"`
	// TestsDir overrides the discovery root for `vigie test`, in every tier.
	// Relative paths resolve against the chart directory. Empty falls back to
	// `<chart>/tests`. The directory is scanned recursively for `*_test.yaml`.
	TestsDir string `yaml:"testsDir" json:"testsDir,omitempty"`
	// Cluster holds the per-backend settings for the cluster tiers.
	Cluster ClusterConfig `yaml:"cluster" json:"cluster,omitempty"`
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
	ApplyTiers []string `yaml:"applyTiers" json:"applyTiers,omitempty"`
}

// ClusterConfig groups the per-backend settings for the cluster tiers of
// `vigie test`. It does not select a backend — `--cluster <backend>` does, and
// only the matching sub-block is read for a given run. Charts can therefore
// pin every backend's settings once and switch tiers from the CLI.
type ClusterConfig struct {
	// Envtest configures the in-process apiserver backend (`--cluster envtest`).
	Envtest EnvtestConfig `yaml:"envtest" json:"envtest,omitempty"`
	// Kind configures the kind backend (`--cluster kind`).
	Kind NodeBackendConfig `yaml:"kind" json:"kind,omitempty"`
	// K3d configures the k3d backend (`--cluster k3d`).
	K3d NodeBackendConfig `yaml:"k3d" json:"k3d,omitempty"`
	// Kubeconfig configures the external-cluster backend (`--cluster kubeconfig`).
	Kubeconfig KubeconfigBackendConfig `yaml:"kubeconfig" json:"kubeconfig,omitempty"`
}

// EnvtestConfig configures the envtest backend, which runs a real
// kube-apiserver and etcd in-process with no controllers.
type EnvtestConfig struct {
	// KubeVersion pins the envtest binary asset version. Empty uses the
	// built-in default. Overridden by `--kube-version`.
	KubeVersion string `yaml:"kubeVersion" json:"kubeVersion,omitempty"`
}

// NodeBackendConfig configures a node-backed backend (kind or k3d), each of
// which provisions a throwaway cluster by driving its external CLI.
type NodeBackendConfig struct {
	// KubeVersion pins the node image version. Empty uses the CLI's default.
	// Overridden by `--kube-version`.
	KubeVersion string `yaml:"kubeVersion" json:"kubeVersion,omitempty"`
	// Binary is the path to the backend's CLI. Empty resolves it from PATH,
	// then the vigie cache, then an optional download. Overridden by
	// `--kind-binary` / `--k3d-binary`.
	Binary string `yaml:"binary" json:"binary,omitempty"`
	// ExtraArgs are additional flags passed verbatim to the provisioning CLI.
	// Example: ["--config", "kind-3node.yaml"] for kind, or
	// ["-v", "/host:/node"] for k3d.
	ExtraArgs []string `yaml:"extraArgs" json:"extraArgs,omitempty"`
}

// KubeconfigBackendConfig configures the kubeconfig backend, which runs against
// a cluster the user already operates.
type KubeconfigBackendConfig struct {
	// Path is the kubeconfig file to reach the cluster with. Mirrors
	// `helm --kubeconfig`. No shell expansion happens, so `~` is not a home
	// directory here. Overridden by `--kubeconfig`.
	Path string `yaml:"path" json:"path,omitempty"`
}

// Defaults holds the values every test inherits unless a suite-level
// `defaults:` or a per-test `inputs:` block overrides them.
type Defaults struct {
	// Release is the release identity passed to `helm template`.
	Release ReleaseDefaults `yaml:"release" json:"release,omitempty"`
}

// ReleaseDefaults is the Helm release identity used to render every test.
type ReleaseDefaults struct {
	// Name is the release name. Mirrors Helm's `--release-name`.
	// Defaults to "release-name".
	Name string `yaml:"name" json:"name,omitempty"`
	// Namespace is the release namespace. Mirrors Helm's `--namespace`.
	// Defaults to "default".
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`
}

// DefaultConfig returns the built-in configuration used when `.vigie.yaml` is
// absent, and as the base every loaded file is decoded on top of.
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
	// The schema runs before the decode: it reports the whole set of violations
	// at once, keyed by JSON pointer, where the decoder stops at the first one.
	if err := Validate(data); err != nil {
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
