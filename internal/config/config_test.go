package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultConfig_ClusterDefaultsAreEmpty(t *testing.T) {
	cfg := DefaultConfig()

	// No backend is pre-configured: `--cluster <backend>` selects the tier and
	// every backend falls back to its own built-in defaults.
	if got, want := cfg.Test.Cluster, (ClusterConfig{}); !reflect.DeepEqual(got, want) {
		t.Errorf("Test.Cluster: want zero value, got %+v", got)
	}
	if len(cfg.Run.ApplyTiers) != 0 {
		t.Errorf("Run.ApplyTiers: want empty, got %v", cfg.Run.ApplyTiers)
	}
}

func TestRunConfig_DefaultEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Run.ApplyTiers) != 0 {
		t.Errorf("DefaultConfig().Run.ApplyTiers: want empty (no apply tiers in `vigie run` unless opted in), got %v", cfg.Run.ApplyTiers)
	}
}

func TestClusterConfig_RoundTrip(t *testing.T) {
	const doc = `
test:
  cluster:
    envtest:
      kubeVersion: "1.30.0"
    kind:
      kubeVersion: "1.31.0"
      binary: /usr/local/bin/kind
      extraArgs:
        - --config
        - kind-3node.yaml
    k3d:
      binary: /usr/local/bin/k3d
    kubeconfig:
      path: /tmp/kubeconfig.yaml
run:
  applyTiers:
    - envtest
    - kind
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	want := ClusterConfig{
		Envtest: EnvtestConfig{KubeVersion: "1.30.0"},
		Kind: NodeBackendConfig{
			KubeVersion: "1.31.0",
			Binary:      "/usr/local/bin/kind",
			ExtraArgs:   []string{"--config", "kind-3node.yaml"},
		},
		K3d:        NodeBackendConfig{Binary: "/usr/local/bin/k3d"},
		Kubeconfig: KubeconfigBackendConfig{Path: "/tmp/kubeconfig.yaml"},
	}
	if got := cfg.Test.Cluster; !reflect.DeepEqual(got, want) {
		t.Errorf("Test.Cluster:\n got %+v\nwant %+v", got, want)
	}
	if got, want := cfg.Run.ApplyTiers, []string{"envtest", "kind"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Run.ApplyTiers: want %v, got %v", want, got)
	}
}

func TestTestsDirConfig_RoundTrip(t *testing.T) {
	const doc = `
test:
  testsDir: ../tests/charts/my-chart-tests
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got, want := cfg.Test.TestsDir, "../tests/charts/my-chart-tests"; got != want {
		t.Errorf("Test.TestsDir: want %q, got %q", want, got)
	}
}

func TestLoad_RetiredTestApplyKeyPointsAtNewHome(t *testing.T) {
	dir := t.TempDir()
	body := "test:\n  testsDir: tests/unit\ntestApply:\n  cluster:\n    type: kind\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load must reject the retired `testApply:` key, got nil error")
	}
	for _, want := range []string{"testApply", "test.cluster.<backend>", "line 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load on chart without .vigie.yaml: %v", err)
	}
	if cfg.Defaults.Release.Name != "release-name" {
		t.Errorf("want default release name, got %q", cfg.Defaults.Release.Name)
	}
}

func TestLoad_EmptyFileKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("\n  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load on empty .vigie.yaml: %v", err)
	}
	if cfg.Defaults.Release.Namespace != "default" {
		t.Errorf("want default namespace, got %q", cfg.Defaults.Release.Namespace)
	}
}

// TestLoad_ExampleConfigIsValid keeps examples/.vigie.yaml honest: the strict
// loader rejects any key the structs no longer have, so the documented
// reference cannot drift away from the config model.
func TestLoad_ExampleConfigIsValid(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("loading examples/.vigie.yaml: %v", err)
	}
	// Spot-check one key per top-level block so an example gutted by accident
	// fails here rather than passing as a valid empty file.
	if cfg.Defaults.Release.Name == "" {
		t.Error("example documents no defaults.release.name")
	}
	if len(cfg.Lint.RuleSets) == 0 {
		t.Error("example documents no lint.ruleSets")
	}
	if cfg.Test.TestsDir == "" {
		t.Error("example documents no test.testsDir")
	}
	if cfg.Test.Cluster.Envtest.KubeVersion == "" {
		t.Error("example documents no test.cluster.envtest.kubeVersion")
	}
	if cfg.Test.Cluster.Kubeconfig.Path == "" {
		t.Error("example documents no test.cluster.kubeconfig.path")
	}
}

func TestLoad_UnknownKeyIsRejected(t *testing.T) {
	dir := t.TempDir()
	// `ruleSet` is a typo for `ruleSets` — strict decoding must surface it
	// instead of silently dropping the user's intent.
	body := "lint:\n  ruleSet:\n    - chart-yaml\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Load must reject an unknown key (misspelled `ruleSet`), got nil error")
	}
}
