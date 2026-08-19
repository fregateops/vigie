package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateKubeVersion(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty is allowed (falls back to default)", "", false},
		{"full semver", "1.36.1", false},
		{"full semver with v prefix", "v1.36.1", false},
		{"zero patch", "1.30.0", false},
		{"two-digit minor", "1.29.10", false},
		{"truncated major.minor rejected", "1.30", true},
		{"truncated major.minor with v rejected", "v1.30", true},
		{"major only rejected", "1", true},
		{"trailing dot rejected", "1.30.", true},
		{"leading zero on minor rejected", "1.030.0", true},
		{"alpha suffix rejected", "1.30.0-alpha", true},
		{"build metadata rejected", "1.30.0+build", true},
		{"non-numeric rejected", "latest", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateKubeVersion("test", tc.value)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateKubeVersion(%q) = nil, want error", tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateKubeVersion(%q) = %v, want nil", tc.value, err)
			}
		})
	}
}

func TestValidateKubeVersion_ErrorMessageMentionsSource(t *testing.T) {
	err := ValidateKubeVersion("--kube-version", "1.30")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--kube-version") {
		t.Errorf("error %q does not mention the source field", err)
	}
	if !strings.Contains(err.Error(), "X.Y.Z") {
		t.Errorf("error %q does not show the expected format", err)
	}
}

func TestLoad_RejectsTruncatedKubeVersionPerClusterBackend(t *testing.T) {
	for _, backend := range []string{"envtest", "kind", "k3d"} {
		t.Run(backend, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, `test:
  cluster:
    `+backend+`:
      kubeVersion: "1.30"
`)
			_, err := Load(dir)
			if err == nil {
				t.Fatal("Load should have rejected truncated kubeVersion, got nil error")
			}
			want := "test.cluster." + backend + ".kubeVersion"
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not point at %s", err, want)
			}
		})
	}
}

func TestLoad_RejectsTruncatedKubeVersionInValidate(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `validate:
  kubeVersions:
    - "1.36.1"
    - "1.30"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load should have rejected truncated kubeVersion, got nil error")
	}
	if !strings.Contains(err.Error(), "validate.kubeVersions[1]") {
		t.Errorf("error %q does not pinpoint the bad list index", err)
	}
}

func TestLoad_AcceptsFullSemverEverywhere(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `validate:
  kubeVersions:
    - "1.36.1"
test:
  kubeVersions:
    - "1.36.1"
  cluster:
    envtest:
      kubeVersion: "1.36.1"
    kind:
      kubeVersion: "1.36.1"
    k3d:
      kubeVersion: "1.36.1"
`)
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoad_AcceptsMajorMinorForLint(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `lint:
  kubeVersions:
    - "1.27"
    - "1.30"
`)
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load should accept MAJOR.MINOR for lint.kubeVersions, got: %v", err)
	}
}

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, ".vigie.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
