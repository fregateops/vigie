package doctor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeBinary writes path with content `#!/bin/sh\nexit 0\n` and the
// requested mode. The parent directory must already exist.
func writeFakeBinary(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatalf("write fake binary %s: %v", path, err)
	}
}

// stubExec replaces the package-level lookPath and runCommand vars for
// the duration of the test.
func stubExec(t *testing.T, look func(string) (string, error), run func(string, ...string) ([]byte, error)) {
	t.Helper()
	origLook := lookPath
	origRun := runCommand
	t.Cleanup(func() {
		lookPath = origLook
		runCommand = origRun
	})
	if look != nil {
		lookPath = look
	}
	if run != nil {
		runCommand = run
	}
}

func TestCheckKubebuilderAssets(t *testing.T) {
	t.Run("env var unset", func(t *testing.T) {
		t.Setenv("KUBEBUILDER_ASSETS", "")

		bin, check := CheckKubebuilderAssets()
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check != nil {
			t.Errorf("want nil check, got %+v", check)
		}
	})

	t.Run("directory does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		t.Setenv("KUBEBUILDER_ASSETS", missing)

		bin, check := CheckKubebuilderAssets()
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check == nil {
			t.Fatalf("want check, got nil")
		}
		if check.Status != StatusError {
			t.Errorf("want status %q, got %q", StatusError, check.Status)
		}
		if !strings.Contains(check.Detail, "does not exist") {
			t.Errorf("detail %q missing 'does not exist'", check.Detail)
		}
	})

	t.Run("missing both binaries", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("KUBEBUILDER_ASSETS", dir)

		bin, check := CheckKubebuilderAssets()
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check == nil {
			t.Fatalf("want check, got nil")
		}
		if check.Status != StatusError {
			t.Errorf("want status %q, got %q", StatusError, check.Status)
		}
		if !strings.Contains(check.Detail, "kube-apiserver") {
			t.Errorf("detail %q should mention kube-apiserver", check.Detail)
		}
		if !strings.Contains(check.Detail, "etcd") {
			t.Errorf("detail %q should mention etcd", check.Detail)
		}
	})

	t.Run("missing only etcd", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeBinary(t, filepath.Join(dir, "kube-apiserver"), 0o755)
		t.Setenv("KUBEBUILDER_ASSETS", dir)

		bin, check := CheckKubebuilderAssets()
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check == nil {
			t.Fatalf("want check, got nil")
		}
		if check.Status != StatusError {
			t.Errorf("want status %q, got %q", StatusError, check.Status)
		}
		if !strings.Contains(check.Detail, "etcd") {
			t.Errorf("detail %q should mention etcd", check.Detail)
		}
		if strings.Contains(check.Detail, "kube-apiserver") {
			t.Errorf("detail %q should NOT mention kube-apiserver", check.Detail)
		}
	})

	t.Run("files exist but not executable", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeBinary(t, filepath.Join(dir, "kube-apiserver"), 0o644)
		writeFakeBinary(t, filepath.Join(dir, "etcd"), 0o644)
		t.Setenv("KUBEBUILDER_ASSETS", dir)

		bin, check := CheckKubebuilderAssets()
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check == nil {
			t.Fatalf("want check, got nil")
		}
		if check.Status != StatusError {
			t.Errorf("want status %q, got %q", StatusError, check.Status)
		}
	})

	t.Run("both binaries executable", func(t *testing.T) {
		dir := t.TempDir()
		apiserver := filepath.Join(dir, "kube-apiserver")
		etcd := filepath.Join(dir, "etcd")
		writeFakeBinary(t, apiserver, 0o755)
		writeFakeBinary(t, etcd, 0o755)
		t.Setenv("KUBEBUILDER_ASSETS", dir)

		bin, check := CheckKubebuilderAssets()
		if check != nil {
			t.Errorf("want nil check, got %+v", check)
		}
		if bin == nil {
			t.Fatalf("want binaries, got nil")
		}
		if bin.APIServerPath != apiserver {
			t.Errorf("APIServerPath = %q, want %q", bin.APIServerPath, apiserver)
		}
		if bin.EtcdPath != etcd {
			t.Errorf("EtcdPath = %q, want %q", bin.EtcdPath, etcd)
		}
		if bin.Source != "KUBEBUILDER_ASSETS" {
			t.Errorf("Source = %q, want %q", bin.Source, "KUBEBUILDER_ASSETS")
		}
	})
}

func TestCheckSetupEnvtest(t *testing.T) {
	t.Run("not on PATH", func(t *testing.T) {
		stubExec(t, func(_ string) (string, error) { return "", exec.ErrNotFound }, nil)

		bin, check := CheckSetupEnvtest("1.30")
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check.Status != StatusWarning {
			t.Errorf("want status %q, got %q", StatusWarning, check.Status)
		}
		if !strings.Contains(check.Detail, "not found") {
			t.Errorf("detail %q missing 'not found'", check.Detail)
		}
	})

	t.Run("use command fails", func(t *testing.T) {
		stubExec(t,
			func(_ string) (string, error) { return "/usr/bin/setup-envtest", nil },
			func(_ string, _ ...string) ([]byte, error) { return nil, errors.New("network down") },
		)

		bin, check := CheckSetupEnvtest("1.30")
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check.Status != StatusWarning {
			t.Errorf("want status %q, got %q", StatusWarning, check.Status)
		}
		if !strings.Contains(check.Detail, "failed") {
			t.Errorf("detail %q missing 'failed'", check.Detail)
		}
	})

	t.Run("resolved path missing executables", func(t *testing.T) {
		emptyDir := t.TempDir()
		stubExec(t,
			func(_ string) (string, error) { return "/usr/bin/setup-envtest", nil },
			func(_ string, _ ...string) ([]byte, error) { return []byte(emptyDir + "\n"), nil },
		)

		bin, check := CheckSetupEnvtest("1.30")
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if check.Status != StatusWarning {
			t.Errorf("want status %q, got %q", StatusWarning, check.Status)
		}
		if !strings.Contains(check.Detail, "missing executables") {
			t.Errorf("detail %q missing 'missing executables'", check.Detail)
		}
	})

	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		apiserver := filepath.Join(dir, "kube-apiserver")
		etcd := filepath.Join(dir, "etcd")
		writeFakeBinary(t, apiserver, 0o755)
		writeFakeBinary(t, etcd, 0o755)

		stubExec(t,
			func(_ string) (string, error) { return "/usr/bin/setup-envtest", nil },
			func(_ string, _ ...string) ([]byte, error) { return []byte(dir + "\n"), nil },
		)

		bin, check := CheckSetupEnvtest("1.30")
		if bin == nil {
			t.Fatalf("want binaries, got nil")
		}
		if bin.APIServerPath != apiserver {
			t.Errorf("APIServerPath = %q, want %q", bin.APIServerPath, apiserver)
		}
		if bin.EtcdPath != etcd {
			t.Errorf("EtcdPath = %q, want %q", bin.EtcdPath, etcd)
		}
		if bin.Source != "setup-envtest (k8s 1.30)" {
			t.Errorf("Source = %q, want %q", bin.Source, "setup-envtest (k8s 1.30)")
		}
		if check.Status != StatusOK {
			t.Errorf("want status %q, got %q", StatusOK, check.Status)
		}
	})
}

func TestLocateEnvtestBinaries(t *testing.T) {
	t.Run("KUBEBUILDER_ASSETS resolves", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeBinary(t, filepath.Join(dir, "kube-apiserver"), 0o755)
		writeFakeBinary(t, filepath.Join(dir, "etcd"), 0o755)
		t.Setenv("KUBEBUILDER_ASSETS", dir)

		// setup-envtest must NOT be consulted on the happy path.
		stubExec(t,
			func(_ string) (string, error) {
				t.Fatal("setup-envtest lookup should not be attempted when KUBEBUILDER_ASSETS resolves")
				return "", nil
			},
			nil,
		)

		bin, trace := LocateEnvtestBinaries("1.30")
		if bin == nil {
			t.Fatalf("want binaries, got nil")
		}
		if bin.Source != "KUBEBUILDER_ASSETS" {
			t.Errorf("Source = %q, want %q", bin.Source, "KUBEBUILDER_ASSETS")
		}
		if len(trace) != 0 {
			t.Errorf("want empty trace on KUBEBUILDER_ASSETS success, got %+v", trace)
		}
	})

	t.Run("setup-envtest fallback", func(t *testing.T) {
		t.Setenv("KUBEBUILDER_ASSETS", "")

		dir := t.TempDir()
		writeFakeBinary(t, filepath.Join(dir, "kube-apiserver"), 0o755)
		writeFakeBinary(t, filepath.Join(dir, "etcd"), 0o755)

		stubExec(t,
			func(_ string) (string, error) { return "/usr/bin/setup-envtest", nil },
			func(_ string, _ ...string) ([]byte, error) { return []byte(dir + "\n"), nil },
		)

		bin, trace := LocateEnvtestBinaries("1.30")
		if bin == nil {
			t.Fatalf("want binaries, got nil")
		}
		if bin.Source != "setup-envtest (k8s 1.30)" {
			t.Errorf("Source = %q, want %q", bin.Source, "setup-envtest (k8s 1.30)")
		}
		if len(trace) != 1 {
			t.Fatalf("want 1 trace entry, got %d: %+v", len(trace), trace)
		}
		if trace[0].Name != "setup-envtest" {
			t.Errorf("trace[0].Name = %q, want %q", trace[0].Name, "setup-envtest")
		}
		if trace[0].Status != StatusOK {
			t.Errorf("trace[0].Status = %q, want %q", trace[0].Status, StatusOK)
		}
	})

	t.Run("KUBEBUILDER_ASSETS invalid + setup-envtest missing", func(t *testing.T) {
		// Bad env var triggers an error trace, then setup-envtest fallback fails.
		t.Setenv("KUBEBUILDER_ASSETS", filepath.Join(t.TempDir(), "nope"))

		stubExec(t, func(_ string) (string, error) { return "", exec.ErrNotFound }, nil)

		bin, trace := LocateEnvtestBinaries("1.30")
		if bin != nil {
			t.Errorf("want nil binaries, got %+v", bin)
		}
		if len(trace) != 2 {
			t.Fatalf("want 2 trace entries (env var error + setup-envtest warning), got %d: %+v", len(trace), trace)
		}
		if trace[0].Name != "KUBEBUILDER_ASSETS" || trace[0].Status != StatusError {
			t.Errorf("trace[0] = %+v, want KUBEBUILDER_ASSETS error", trace[0])
		}
		if trace[1].Name != "setup-envtest" || trace[1].Status != StatusWarning {
			t.Errorf("trace[1] = %+v, want setup-envtest warning", trace[1])
		}
	})
}
