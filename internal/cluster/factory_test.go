package cluster_test

import (
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/cluster"
)

func TestNew_envtest(t *testing.T) {
	backend, err := cluster.New(cluster.Config{Type: "envtest"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestNew_defaultIsEnvtest(t *testing.T) {
	backend, err := cluster.New(cluster.Config{})
	if err != nil {
		t.Fatalf("unexpected error for empty type: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestNew_kubeconfig(t *testing.T) {
	backend, err := cluster.New(cluster.Config{Type: "kubeconfig", Kubeconfig: "/some/kubeconfig"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestNew_kubeconfig_requiresPath(t *testing.T) {
	_, err := cluster.New(cluster.Config{Type: "kubeconfig"})
	if err == nil {
		t.Fatal("expected error when kubeconfig path is empty")
	}
	if !strings.Contains(err.Error(), "kubeconfig path") {
		t.Fatalf("error should mention the missing kubeconfig path: %v", err)
	}
}

func TestNew_kind(t *testing.T) {
	backend, err := cluster.New(cluster.Config{Type: "kind"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestNew_unknown(t *testing.T) {
	_, err := cluster.New(cluster.Config{Type: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}
