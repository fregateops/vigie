//go:build integration

package runner

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/config"
)

// TestRunApply_NodeBackends provisions a real node-backed cluster (kind, k3d)
// and runs the apply tier end-to-end over the basic chart, asserting the run
// installs and asserts without failures. Each backend subtest skips when its
// CLI or a container runtime is unavailable, so `go test -tags integration`
// stays green on machines that only have some of the tooling.
//
// These are slow (a cluster is provisioned per subtest) and network-bound
// (node images are pulled), so they live behind the integration build tag and
// out of the default `make test`.
func TestRunApply_NodeBackends(t *testing.T) {
	backends := []string{"kind", "k3d"}
	chartPath := filepath.Join("..", "..", "testdata", "charts", "basic")
	testFile := filepath.Join(chartPath, "tests", "unit", "deployment_test.yaml")

	for _, backendType := range backends {
		t.Run(backendType, func(t *testing.T) {
			skipUnlessNodeToolingPresent(t, backendType)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			t.Cleanup(cancel)

			backend, err := cluster.New(cluster.Config{Type: backendType})
			if err != nil {
				t.Fatalf("cluster.New(%s): %v", backendType, err)
			}

			results, err := RunApply(ctx, ApplyOptions{
				ChartPath:   chartPath,
				TestFiles:   []string{testFile},
				Parallelism: 1,
				Cfg:         config.DefaultConfig(),
				Backend:     backend,
				BackendType: backendType,
			})
			if err != nil {
				t.Fatalf("RunApply(%s): %v", backendType, err)
			}

			total := 0
			for _, sr := range results {
				total += len(sr.Results)
			}
			if total == 0 {
				t.Fatalf("%s: expected at least one test result", backendType)
			}
			if AnyFailed(results) {
				t.Fatalf("%s: RunApply reported failures; see logged details above", backendType)
			}
		})
	}
}

// skipUnlessNodeToolingPresent skips when the backend's CLI or a container
// runtime is missing - both are prerequisites for provisioning a real cluster.
func skipUnlessNodeToolingPresent(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s not found in PATH; skipping %s apply integration test", tool, tool)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		if _, podmanErr := exec.LookPath("podman"); podmanErr != nil {
			t.Skipf("neither docker nor podman found in PATH; skipping %s apply integration test", tool)
		}
	}
}
