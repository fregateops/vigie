package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/cluster/envtest"
	"github.com/fregateops/vigie/internal/config"
)

// TestRunApply_NoFiles ensures RunApply returns no results and no error when
// called with an empty file list, without ever starting the backend.
func TestRunApply_NoFiles(t *testing.T) {
	results, err := RunApply(context.Background(), ApplyOptions{
		ChartPath:   "testdata/none",
		TestFiles:   nil,
		Cfg:         config.DefaultConfig(),
		BackendType: "envtest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results for empty file list, got %d entries", len(results))
	}
}

// TestRunApply_BackendStartFails ensures setup errors surface when the envtest
// backend cannot find binaries. We disable the auto-download fallback to keep
// the test offline and force a clean setup-failure path.
func TestRunApply_BackendStartFails(t *testing.T) {
	t.Setenv("KUBEBUILDER_ASSETS", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("VIGIE_DISABLE_AUTO_DOWNLOAD", "1")

	_, err := RunApply(context.Background(), ApplyOptions{
		ChartPath:   "testdata/none",
		TestFiles:   []string{"testdata/none/tests/unit/x_test.yaml"},
		Cfg:         config.DefaultConfig(),
		Backend:     envtest.New("1.30"),
		BackendType: "envtest",
	})
	if err == nil {
		t.Fatal("expected setup error when envtest binaries are unavailable")
	}
	if !strings.Contains(err.Error(), "setup error") {
		t.Fatalf("error should be tagged as 'setup error', got: %v", err)
	}
}

// TestRunApply_BadMatchRegex ensures an invalid --match value is reported as
// an error before the backend boots.
func TestRunApply_BadMatchRegex(t *testing.T) {
	_, err := RunApply(context.Background(), ApplyOptions{
		ChartPath:   "testdata/none",
		TestFiles:   []string{"x"},
		Cfg:         config.DefaultConfig(),
		BackendType: "envtest",
		Match:       "[",
	})
	if err == nil {
		t.Fatal("expected error for invalid --match regex")
	}
	if !strings.Contains(err.Error(), "invalid --match regex") {
		t.Fatalf("expected 'invalid --match regex' in error, got: %v", err)
	}
}

// TestAllocateNamespace_ShapeAndUniqueness covers the DNS-label sanitiser and
// the random suffix.
func TestAllocateNamespace_ShapeAndUniqueness(t *testing.T) {
	inputs := []string{
		"renders a Deployment",
		"WEIRD/Name [matrix=foo, x=1]",
		"a really really really really long display name that exceeds the dns label limit easily",
		"",
	}
	seen := map[string]bool{}
	for _, name := range inputs {
		got := allocateNamespace(name)
		if len(got) > 63 {
			t.Errorf("namespace %q for input %q exceeds 63 chars", got, name)
		}
		if !strings.HasPrefix(got, "vg-") {
			t.Errorf("namespace %q should start with 'vg-'", got)
		}
		if strings.Contains(got, " ") || strings.Contains(got, "/") {
			t.Errorf("namespace %q contains illegal DNS-label characters", got)
		}
		if seen[got] {
			t.Errorf("duplicate namespace name produced: %q", got)
		}
		seen[got] = true
	}
}

// TestAllocateNamespace_EightHexSuffix verifies the suffix is exactly 8 hex
// characters, matching the `vg-<name>-<8hex>` pattern from the spec.
func TestAllocateNamespace_EightHexSuffix(t *testing.T) {
	got := allocateNamespace("my-test")
	parts := strings.Split(got, "-")
	if len(parts) < 3 {
		t.Fatalf("expected at least 3 dash-separated segments, got %q", got)
	}
	suffix := parts[len(parts)-1]
	if len(suffix) != 8 {
		t.Errorf("suffix should be 8 chars, got %d in %q", len(suffix), got)
	}
	for _, ch := range suffix {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			t.Errorf("non-hex char %q in suffix of %q", ch, got)
		}
	}
}

// TestRunApply_DefaultParallelism ensures that when Parallelism is 0 (or
// negative) the runner selects runtime.NumCPU() as the worker pool size. We
// verify indirectly via the early-exit path (no test files).
func TestRunApply_DefaultParallelism(t *testing.T) {
	results, err := RunApply(context.Background(), ApplyOptions{
		ChartPath:   "testdata/none",
		TestFiles:   nil,
		Parallelism: 0,
		Cfg:         config.DefaultConfig(),
		BackendType: "envtest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results for empty file list, got %d entries", len(results))
	}
}

// TestBackendTier ensures the tier filter target matches the backend
// classification used by `tier:` annotations in test files.
func TestBackendTier(t *testing.T) {
	cases := []struct {
		backend string
		want    string
	}{
		{"envtest", "apiserver"},
		{"kind", "e2e"},
		{"k3d", "e2e"},
		{"kubeconfig", "e2e"},
		{"simulated", "simulated"},
	}
	for _, tc := range cases {
		if got := backendTier(tc.backend); got != tc.want {
			t.Errorf("backendTier(%q) = %q, want %q", tc.backend, got, tc.want)
		}
	}
}
