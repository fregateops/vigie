package deps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestResolveRefs_noRefs(t *testing.T) {
	input := []dsl.Dependency{
		{Name: "alpha", Source: dsl.DependencySource{Manifest: "alpha.yaml"}},
		{Name: "beta", Source: dsl.DependencySource{Manifest: "beta.yaml"}},
	}
	result, err := ResolveRefs(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(result))
	}
}

func TestResolveRefs_singleRef(t *testing.T) {
	dir := t.TempDir()
	refFile := filepath.Join(dir, "shared-deps.yaml")
	content := `
dependencies:
  - name: redis
    namespace: redis
    source:
      manifest: redis.yaml
`
	if err := os.WriteFile(refFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	input := []dsl.Dependency{
		{Name: "my-ref", Source: dsl.DependencySource{Ref: refFile}},
	}
	result, err := ResolveRefs(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 dep from ref, got %d", len(result))
	}
	if result[0].Name != "redis" {
		t.Errorf("expected dep named redis, got %q", result[0].Name)
	}
}

func TestResolveRefs_refMixed(t *testing.T) {
	dir := t.TempDir()
	refFile := filepath.Join(dir, "infra.yaml")
	content := `
dependencies:
  - name: db
    namespace: db
    source:
      manifest: db.yaml
  - name: cache
    namespace: cache
    source:
      manifest: cache.yaml
`
	if err := os.WriteFile(refFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	input := []dsl.Dependency{
		{Name: "app", Source: dsl.DependencySource{Manifest: "app.yaml"}},
		{Name: "infra-ref", Source: dsl.DependencySource{Ref: refFile}},
	}
	result, err := ResolveRefs(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 non-ref + 2 inlined from ref = 3 total
	if len(result) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(result))
	}
	if result[0].Name != "app" {
		t.Errorf("expected app first, got %q", result[0].Name)
	}
}

func TestResolveRefs_missingFile(t *testing.T) {
	input := []dsl.Dependency{
		{Name: "bad-ref", Source: dsl.DependencySource{Ref: "/nonexistent/path.yaml"}},
	}
	_, err := ResolveRefs(input)
	if err == nil {
		t.Fatal("expected error for missing ref file, got nil")
	}
}

func TestResolveRefs_maxDepth(t *testing.T) {
	dir := t.TempDir()

	// Build a chain of ref files each pointing to the next.
	const chainLen = maxRefDepth + 2
	files := make([]string, chainLen)
	for idx := 0; idx < chainLen; idx++ {
		files[idx] = filepath.Join(dir, "dep"+string(rune('0'+idx))+".yaml")
	}

	// Last file has a real dep.
	last := `
dependencies:
  - name: leaf
    source:
      manifest: leaf.yaml
`
	if err := os.WriteFile(files[chainLen-1], []byte(last), 0o644); err != nil {
		t.Fatal(err)
	}

	// Each preceding file refs the next.
	for idx := chainLen - 2; idx >= 0; idx-- {
		content := "dependencies:\n  - name: ref" + string(rune('0'+idx)) + "\n    source:\n      ref: " + files[idx+1] + "\n"
		if err := os.WriteFile(files[idx], []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	input := []dsl.Dependency{
		{Name: "start", Source: dsl.DependencySource{Ref: files[0]}},
	}
	_, err := ResolveRefs(input)
	if err == nil {
		t.Fatal("expected max-depth error, got nil")
	}
}
