package kubeconfig_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fregateops/vigie/internal/cluster/kubeconfig"
)

// TestNew returns an unstarted backend with the given path.
func TestNew(t *testing.T) {
	backend := kubeconfig.New("/path/to/kubeconfig")
	if backend == nil {
		t.Fatal("New returned nil")
	}
	if backend.Kubeconfig() != "/path/to/kubeconfig" {
		t.Fatalf("expected kubeconfig path %q, got %q", "/path/to/kubeconfig", backend.Kubeconfig())
	}
	if backend.RESTConfig() != nil {
		t.Fatal("expected nil RESTConfig before Start")
	}
}

// TestStart_BadKubeconfigPath verifies that Start returns an error when the
// kubeconfig file does not exist, without requiring a real cluster.
func TestStart_BadKubeconfigPath(t *testing.T) {
	backend := kubeconfig.New("/nonexistent/path/to/kubeconfig.yaml")
	err := backend.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to fail with a missing kubeconfig file")
	}
	if backend.RESTConfig() != nil {
		t.Fatal("expected nil RESTConfig after failed Start")
	}
}

// TestStop_NoOp verifies that Stop always returns nil regardless of whether
// Start was called.
func TestStop_NoOp(t *testing.T) {
	backend := kubeconfig.New("/some/kubeconfig")
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatalf("Stop should be a no-op, got: %v", err)
	}
}

// TestLoadImages_NotSupported verifies that LoadImages returns a descriptive error.
func TestLoadImages_NotSupported(t *testing.T) {
	backend := kubeconfig.New("/some/kubeconfig")
	err := backend.LoadImages(context.Background(), []string{"nginx:latest"})
	if err == nil {
		t.Fatal("expected LoadImages to return an error for external clusters")
	}
	if !errors.Is(err, kubeconfig.ErrImageLoadNotSupported) {
		t.Fatalf("expected ErrImageLoadNotSupported, got: %v", err)
	}
}
