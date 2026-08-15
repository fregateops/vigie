//go:build integration

package kind_test

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/fregateops/vigie/internal/cluster/kind"
)

func TestKindBackend_StartApplyStop(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		if _, podmanErr := exec.LookPath("podman"); podmanErr != nil {
			t.Skip("neither docker nor podman found in PATH; skipping kind integration test")
		}
	}

	clusterName := fmt.Sprintf("vigie-kind-%x", rand.Uint32())
	backend := kind.New(clusterName, "", nil)
	ctx := context.Background()

	if err := backend.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer backend.Stop(ctx) //nolint:errcheck

	restCfg := backend.RESTConfig()
	if restCfg == nil {
		t.Fatal("RESTConfig() returned nil after Start")
	}
	if backend.Kubeconfig() == "" {
		t.Fatal("Kubeconfig() returned empty string after Start")
	}

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("creating kubernetes client: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vigie-probe",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}
	if _, createErr := client.CoreV1().ConfigMaps("default").Create(ctx, cm, metav1.CreateOptions{}); createErr != nil {
		t.Fatalf("creating ConfigMap: %v", createErr)
	}

	if err := backend.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if backend.RESTConfig() != nil {
		t.Error("RESTConfig() should return nil after Stop")
	}
	if backend.Kubeconfig() != "" {
		t.Error("Kubeconfig() should return empty string after Stop")
	}
}

func TestKindBackend_StopIdempotent(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		if _, podmanErr := exec.LookPath("podman"); podmanErr != nil {
			t.Skip("neither docker nor podman found in PATH; skipping kind integration test")
		}
	}

	clusterName := fmt.Sprintf("vigie-kind-%x", rand.Uint32())
	backend := kind.New(clusterName, "", nil)
	ctx := context.Background()

	// Stop without Start should be safe.
	if err := backend.Stop(ctx); err != nil {
		t.Fatalf("Stop on unstarted backend: %v", err)
	}
}
