package envtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/doctor"
)

// TestStart_BinariesNotFound exercises the path where neither
// KUBEBUILDER_ASSETS nor setup-envtest can produce a working binary pair.
// Start must return an error wrapping ErrBinariesNotFound and pointing at
// `vigie doctor`.
func TestStart_BinariesNotFound(t *testing.T) {
	// Force the env-var path to fail (unset) and the setup-envtest fallback
	// to fail (empty PATH directory contains no binary). Disable
	// auto-download so the test never reaches the network.
	t.Setenv("KUBEBUILDER_ASSETS", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("VIGIE_DISABLE_AUTO_DOWNLOAD", "1")

	b := New("1.30")
	err := b.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to fail when no binaries are discoverable")
	}
	if !errors.Is(err, ErrBinariesNotFound) {
		t.Fatalf("error does not wrap ErrBinariesNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "vigie doctor") {
		t.Fatalf("error message should mention 'vigie doctor', got: %v", err)
	}
	if cfg := b.RESTConfig(); cfg != nil {
		t.Fatalf("expected nil RESTConfig after failed Start, got %+v", cfg)
	}
	// Stop must be a no-op when Start never produced an environment.
	if stopErr := b.Stop(context.Background()); stopErr != nil {
		t.Fatalf("Stop after failed Start should be a no-op, got: %v", stopErr)
	}
}

// TestNew_DefaultsKubeVersion verifies New("") falls back to the default.
func TestNew_DefaultsKubeVersion(t *testing.T) {
	b := New("")
	if b.kubeVersion != doctor.DefaultKubeVersion {
		t.Fatalf("expected default kubeVersion %q, got %q", doctor.DefaultKubeVersion, b.kubeVersion)
	}
}
