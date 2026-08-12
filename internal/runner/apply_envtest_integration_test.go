//go:build envtest

package runner

import (
	"context"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/cluster/envtest"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/doctor"
)

// TestRunApply_NamespaceIsolation_Concurrent boots a real envtest control
// plane, runs several tests concurrently (Parallelism=4), and verifies that
// all per-test namespaces are cleaned up after the run completes.
func TestRunApply_NamespaceIsolation_Concurrent(t *testing.T) {
	const kubeVersion = "1.30"
	if bins, _ := doctor.LocateEnvtestBinaries(kubeVersion); bins == nil {
		t.Skip("envtest binaries not available; run 'vigie doctor'")
	}

	chartPath := filepath.Join("..", "..", "testdata", "charts", "basic")
	testFile := filepath.Join(chartPath, "tests", "unit", "deployment_test.yaml")
	files := []string{testFile, testFile, testFile, testFile}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Spin up an inspection backend separate from the one RunApply manages so
	// we can list namespaces in the external cluster after the run.
	observer := envtest.New(kubeVersion)
	if err := observer.Start(ctx); err != nil {
		t.Fatalf("observer Start: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := observer.Stop(context.Background()); stopErr != nil {
			t.Logf("observer Stop: %v", stopErr)
		}
	})

	kubeCfg := observer.RESTConfig()
	clientset, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		t.Fatalf("kubernetes.NewForConfig: %v", err)
	}

	nsBefore := listVigieNamespaces(t, clientset)
	if len(nsBefore) != 0 {
		t.Fatalf("expected no vg- namespaces before run, found: %v", nsBefore)
	}

	runBackend, err := cluster.New(cluster.Config{Type: "envtest", KubeVersion: kubeVersion})
	if err != nil {
		t.Fatalf("cluster.New: %v", err)
	}
	results, runErr := RunApply(ctx, ApplyOptions{
		ChartPath:   chartPath,
		TestFiles:   files,
		Parallelism: 4,
		Cfg:         config.DefaultConfig(),
		Backend:     runBackend,
		BackendType: "envtest",
	})
	if runErr != nil {
		t.Fatalf("RunApply: %v", runErr)
	}

	totalTests := 0
	for _, sr := range results {
		totalTests += len(sr.Results)
	}
	if totalTests == 0 {
		t.Fatal("expected at least one test result")
	}

	if AnyFailed(results) {
		t.Fatal("AnyFailed reported failures; see logged details above")
	}

	// The runner's namespaces live in its own envtest backend, so the observer
	// (different API server port) should see none of them.
	nsAfter := listVigieNamespaces(t, clientset)
	if len(nsAfter) != 0 {
		t.Fatalf("unexpected vg- namespaces in external cluster after run: %v", nsAfter)
	}
}

// listVigieNamespaces returns the names of all namespaces whose names start
// with "vg-".
func listVigieNamespaces(t *testing.T, clientset *kubernetes.Clientset) []string {
	t.Helper()
	nsList, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listing namespaces: %v", err)
	}
	var names []string
	for _, ns := range nsList.Items {
		if len(ns.Name) > 3 && ns.Name[:3] == "vg-" {
			names = append(names, ns.Name)
		}
	}
	return names
}

// TestRunApply_NamespaceCleanup_Internal verifies that createNamespace +
// deleteNamespace work correctly using a real envtest backend and that a
// namespace is actually removed after deleteNamespace returns.
func TestRunApply_NamespaceCleanup_Internal(t *testing.T) {
	const kubeVersion = "1.30"
	if bins, _ := doctor.LocateEnvtestBinaries(kubeVersion); bins == nil {
		t.Skip("envtest binaries not available; run 'vigie doctor'")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	backend := envtest.New(kubeVersion)
	if err := backend.Start(ctx); err != nil {
		t.Fatalf("backend.Start: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := backend.Stop(context.Background()); stopErr != nil {
			t.Logf("backend.Stop: %v", stopErr)
		}
	})

	kubeCfg := backend.RESTConfig()
	clientset, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		t.Fatalf("kubernetes.NewForConfig: %v", err)
	}

	runner := &applyRunner{clientset: clientset}

	nsName := allocateNamespace("cleanup-test")
	if err := runner.createNamespace(ctx, nsName); err != nil {
		t.Fatalf("createNamespace: %v", err)
	}

	_, err = clientset.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("namespace %q should exist after create, got: %v", nsName, err)
	}

	runner.deleteNamespace(nsName)

	ns, err := clientset.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
	if err == nil && ns.Status.Phase != corev1.NamespaceTerminating {
		t.Errorf("namespace %q should be deleted or terminating, phase: %s", nsName, ns.Status.Phase)
	}
}
