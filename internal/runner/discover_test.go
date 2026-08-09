package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFile creates a test fixture under dir with the given relative path and
// contents, materialising parent directories as needed.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
	return abs
}

const (
	unitSuiteYAML = `suite: u
templates: [templates/x.yaml]
tests:
  - it: ok
    asserts:
      - exists: { path: kind }
`

	integrationSuiteYAML = `suite: i
cluster:
  backend: kind
tests:
  - it: ok
    asserts:
      - exists: { path: kind }
`
)

func TestDiscoverTestFiles_DefaultRoot_PicksUnitOnly(t *testing.T) {
	chart := t.TempDir()
	unitPath := writeFile(t, chart, "tests/unit/deployment_test.yaml", unitSuiteYAML)
	writeFile(t, chart, "tests/integration/e2e_test.yaml", integrationSuiteYAML)
	// Sibling non-test files and snapshots must be ignored.
	writeFile(t, chart, "tests/fixtures/values.yaml", "key: value\n")
	writeFile(t, chart, "tests/snapshots/foo.snap.yaml", "k: v\n")

	got, err := DiscoverTestFiles(chart, "")
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	want := []string{unitPath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverTestFiles default root: want %v, got %v", want, got)
	}
}

func TestDiscoverTestFiles_RecursiveFlatLayout(t *testing.T) {
	chart := t.TempDir()
	rootTest := writeFile(t, chart, "tests/deployment_test.yaml", unitSuiteYAML)
	nestedTest := writeFile(t, chart, "tests/services/ingress_test.yaml", unitSuiteYAML)

	got, err := DiscoverTestFiles(chart, "")
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	want := []string{rootTest, nestedTest}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverTestFiles recursive: want %v, got %v", want, got)
	}
}

func TestDiscoverTestFiles_RelativeTestsDir(t *testing.T) {
	chart := t.TempDir()
	// Sibling tree: chartDir/../sibling-tests/...
	external := filepath.Join(filepath.Dir(chart), "sibling-tests")
	t.Cleanup(func() {
		if err := os.RemoveAll(external); err != nil {
			t.Logf("cleanup %s: %v", external, err)
		}
	})
	unitPath := writeFile(t, external, "deployment_test.yaml", unitSuiteYAML)

	rel, err := filepath.Rel(chart, external)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	got, err := DiscoverTestFiles(chart, rel)
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	// The resolved root is `chart/../sibling-tests`; the walker reports paths
	// as joined with that prefix, not canonicalised, so build the expectation
	// the same way.
	wantPath := filepath.Join(chart, rel, "deployment_test.yaml")
	if len(got) != 1 || got[0] != wantPath {
		// Fall back: accept the cleaned absolute path too — the test should be
		// robust to either form.
		if len(got) != 1 || got[0] != unitPath {
			t.Fatalf("DiscoverTestFiles relative: want %v or %v, got %v", wantPath, unitPath, got)
		}
	}
}

func TestDiscoverTestFiles_AbsoluteTestsDir(t *testing.T) {
	chart := t.TempDir()
	external := t.TempDir()
	unitPath := writeFile(t, external, "deployment_test.yaml", unitSuiteYAML)

	got, err := DiscoverTestFiles(chart, external)
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	want := []string{unitPath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverTestFiles absolute testsDir: want %v, got %v", want, got)
	}
}

func TestDiscoverTestFiles_MissingDirReturnsEmpty(t *testing.T) {
	chart := t.TempDir()
	got, err := DiscoverTestFiles(chart, "")
	if err != nil {
		t.Fatalf("DiscoverTestFiles on missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

func TestDiscoverTestFiles_IgnoresNonTestSuffix(t *testing.T) {
	chart := t.TempDir()
	writeFile(t, chart, "tests/unit/helpers.yaml", unitSuiteYAML)
	writeFile(t, chart, "tests/unit/_test.yaml.bak", unitSuiteYAML)
	wanted := writeFile(t, chart, "tests/unit/deployment_test.yaml", unitSuiteYAML)

	got, err := DiscoverTestFiles(chart, "")
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	if len(got) != 1 || got[0] != wanted {
		t.Fatalf("want exactly [%s], got %v", wanted, got)
	}
}
