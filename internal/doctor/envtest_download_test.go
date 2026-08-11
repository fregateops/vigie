package doctor

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureEnvtestBinaries_LocateSucceeds(t *testing.T) {
	// KUBEBUILDER_ASSETS resolves - auto-download must not be touched.
	dir := t.TempDir()
	apiserver := filepath.Join(dir, "kube-apiserver")
	etcd := filepath.Join(dir, "etcd")
	writeFakeBinary(t, apiserver, 0o755)
	writeFakeBinary(t, etcd, 0o755)
	t.Setenv("KUBEBUILDER_ASSETS", dir)

	stubExec(t,
		func(_ string) (string, error) {
			t.Fatal("setup-envtest lookup should not be attempted when KUBEBUILDER_ASSETS resolves")
			return "", nil
		},
		nil,
	)

	bin, err := EnsureEnvtestBinaries(context.Background(), "1.30", nil)
	if err != nil {
		t.Fatalf("EnsureEnvtestBinaries returned error: %v", err)
	}
	if bin == nil {
		t.Fatal("expected binaries, got nil")
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
}

func TestEnsureEnvtestBinaries_DisabledViaEnv(t *testing.T) {
	// Locate must fail and the kill-switch must skip auto-download.
	t.Setenv("KUBEBUILDER_ASSETS", "")
	stubExec(t, func(_ string) (string, error) { return "", exec.ErrNotFound }, nil)
	t.Setenv(EnvDisableAutoDownload, "1")

	bin, err := EnsureEnvtestBinaries(context.Background(), "1.30", nil)
	if bin != nil {
		t.Fatalf("expected nil binaries, got %+v", bin)
	}
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrAutoDownloadUnsupported) {
		t.Fatalf("error does not wrap ErrAutoDownloadUnsupported: %v", err)
	}
	if !strings.Contains(err.Error(), EnvDisableAutoDownload) {
		t.Errorf("error %q should mention %s", err, EnvDisableAutoDownload)
	}
}
