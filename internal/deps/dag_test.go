package deps

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestBuildBatches_empty(t *testing.T) {
	batches, err := BuildBatches(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("expected empty batches, got %d", len(batches))
	}
}

func TestBuildBatches_single(t *testing.T) {
	deps := []dsl.Dependency{{Name: "alpha"}}
	batches, err := BuildBatches(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if len(batches[0]) != 1 || batches[0][0].Name != "alpha" {
		t.Errorf("unexpected batch content: %v", batches[0])
	}
}

func TestBuildBatches_independent(t *testing.T) {
	deps := []dsl.Dependency{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}
	batches, err := BuildBatches(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch (all independent), got %d", len(batches))
	}
	if len(batches[0]) != 3 {
		t.Errorf("expected 3 items in batch, got %d", len(batches[0]))
	}
}

func TestBuildBatches_chain(t *testing.T) {
	// alpha → beta → gamma (serial chain)
	deps := []dsl.Dependency{
		{Name: "gamma", DependsOn: []string{"beta"}},
		{Name: "alpha"},
		{Name: "beta", DependsOn: []string{"alpha"}},
	}
	batches, err := BuildBatches(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches for a serial chain, got %d", len(batches))
	}
	if batches[0][0].Name != "alpha" {
		t.Errorf("batch 0 should be alpha, got %q", batches[0][0].Name)
	}
	if batches[1][0].Name != "beta" {
		t.Errorf("batch 1 should be beta, got %q", batches[1][0].Name)
	}
	if batches[2][0].Name != "gamma" {
		t.Errorf("batch 2 should be gamma, got %q", batches[2][0].Name)
	}
}

func TestBuildBatches_diamond(t *testing.T) {
	// alpha is root; beta and gamma both depend on alpha; delta depends on both beta and gamma.
	deps := []dsl.Dependency{
		{Name: "alpha"},
		{Name: "beta", DependsOn: []string{"alpha"}},
		{Name: "gamma", DependsOn: []string{"alpha"}},
		{Name: "delta", DependsOn: []string{"beta", "gamma"}},
	}
	batches, err := BuildBatches(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect: [alpha], [beta, gamma], [delta]
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches for diamond, got %d", len(batches))
	}
	if len(batches[0]) != 1 || batches[0][0].Name != "alpha" {
		t.Errorf("batch 0 should be [alpha], got %v", batchNames(batches[0]))
	}
	if len(batches[1]) != 2 {
		t.Errorf("batch 1 should have 2 items (beta, gamma), got %v", batchNames(batches[1]))
	}
	if len(batches[2]) != 1 || batches[2][0].Name != "delta" {
		t.Errorf("batch 2 should be [delta], got %v", batchNames(batches[2]))
	}
}

func TestBuildBatches_cycle(t *testing.T) {
	deps := []dsl.Dependency{
		{Name: "alpha", DependsOn: []string{"beta"}},
		{Name: "beta", DependsOn: []string{"alpha"}},
	}
	_, err := BuildBatches(deps)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestBuildBatches_unknownRef(t *testing.T) {
	deps := []dsl.Dependency{
		{Name: "alpha", DependsOn: []string{"nonexistent"}},
	}
	_, err := BuildBatches(deps)
	if err == nil {
		t.Fatal("expected unknown-ref error, got nil")
	}
}

func TestBuildBatches_selfRef(t *testing.T) {
	deps := []dsl.Dependency{
		{Name: "alpha", DependsOn: []string{"alpha"}},
	}
	_, err := BuildBatches(deps)
	if err == nil {
		t.Fatal("expected cycle error for self-reference, got nil")
	}
}

func batchNames(batch Batch) []string {
	names := make([]string, len(batch))
	for idx, dep := range batch {
		names[idx] = dep.Name
	}
	return names
}
