package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultConfig_TestApplyDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if got, want := cfg.TestApply.Cluster.Type, "envtest"; got != want {
		t.Errorf("TestApply.Cluster.Type: want %q, got %q", want, got)
	}
	if cfg.TestApply.Cluster.KubeVersion != "" {
		t.Errorf("TestApply.Cluster.KubeVersion: want %q, got %q", "", cfg.TestApply.Cluster.KubeVersion)
	}
	if cfg.TestApply.Cluster.Kubeconfig != "" {
		t.Errorf("TestApply.Cluster.Kubeconfig: want %q, got %q", "", cfg.TestApply.Cluster.Kubeconfig)
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

func TestTestApplyConfig_RoundTrip(t *testing.T) {
	const doc = `
testApply:
  cluster:
    type: kubeconfig
    kubeVersion: "1.30.0"
    kubeconfig: /tmp/kubeconfig.yaml
run:
  applyTiers:
    - envtest
    - kind
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if got, want := cfg.TestApply.Cluster.Type, "kubeconfig"; got != want {
		t.Errorf("TestApply.Cluster.Type: want %q, got %q", want, got)
	}
	if got, want := cfg.TestApply.Cluster.KubeVersion, "1.30.0"; got != want {
		t.Errorf("TestApply.Cluster.KubeVersion: want %q, got %q", want, got)
	}
	if got, want := cfg.TestApply.Cluster.Kubeconfig, "/tmp/kubeconfig.yaml"; got != want {
		t.Errorf("TestApply.Cluster.Kubeconfig: want %q, got %q", want, got)
	}
	if got, want := cfg.Run.ApplyTiers, []string{"envtest", "kind"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Run.ApplyTiers: want %v, got %v", want, got)
	}
}

func TestTestsDirConfig_RoundTrip(t *testing.T) {
	const doc = `
test:
  testsDir: ../tests/charts/my-chart-tests
testApply:
  testsDir: /abs/path/tests
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got, want := cfg.Test.TestsDir, "../tests/charts/my-chart-tests"; got != want {
		t.Errorf("Test.TestsDir: want %q, got %q", want, got)
	}
	if got, want := cfg.TestApply.TestsDir, "/abs/path/tests"; got != want {
		t.Errorf("TestApply.TestsDir: want %q, got %q", want, got)
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
