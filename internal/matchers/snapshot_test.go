package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/snapshot"
)

func newStore(t *testing.T, update bool) *snapshot.Store {
	t.Helper()
	return &snapshot.Store{Dir: t.TempDir(), Update: update}
}

func TestEvalMatchSnapshot_NoStore(t *testing.T) {
	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, EvalContext{SnapshotStore: nil})
	if r.Pass {
		t.Error("expected fail when no store configured")
	}
	if r.Message == "" {
		t.Error("expected non-empty failure message")
	}
}

func TestEvalMatchSnapshot_WholeDoc(t *testing.T) {
	doc := map[string]any{"kind": "Deployment", "replicas": 3}
	store := newStore(t, false)
	ctx := EvalContext{
		Doc:           doc,
		SnapshotStore: store,
		SuiteName:     "my-suite",
		TestName:      "my-test",
		AssertIdx:     0,
	}

	// First call: snapshot does not exist — writes it and passes.
	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("first call should pass (writes snapshot): %s", r.Message)
	}

	// Second call: snapshot exists and matches.
	r = evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("second call should pass (matches snapshot): %s", r.Message)
	}
}

func TestEvalMatchSnapshot_Mismatch(t *testing.T) {
	store := newStore(t, false)
	ctx := EvalContext{
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "test",
		AssertIdx:     0,
	}

	// Write initial snapshot.
	ctx.Doc = map[string]any{"kind": "Deployment"}
	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("write pass: %s", r.Message)
	}

	// Change doc — should mismatch.
	ctx.Doc = map[string]any{"kind": "Service"}
	r = evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if r.Pass {
		t.Error("expected mismatch to fail")
	}
	if r.Message == "" {
		t.Error("expected non-empty mismatch message")
	}
}

func TestEvalMatchSnapshot_Update(t *testing.T) {
	doc1 := map[string]any{"kind": "Deployment"}
	doc2 := map[string]any{"kind": "Service"}
	store := newStore(t, false)
	ctx := EvalContext{
		Doc:           doc1,
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "test",
		AssertIdx:     0,
	}

	// Write initial snapshot.
	evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)

	// Update mode: overwrite with new doc — should pass.
	store.Update = true
	ctx.Doc = doc2
	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("update should pass: %s", r.Message)
	}

	// Verify the snapshot now matches doc2.
	store.Update = false
	r = evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("after update, doc2 should match: %s", r.Message)
	}
}

func TestEvalMatchSnapshot_PathBased(t *testing.T) {
	doc := map[string]any{
		"kind": "Deployment",
		"spec": map[string]any{"replicas": 3},
	}
	store := newStore(t, false)
	ctx := EvalContext{
		Doc:           doc,
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "path-test",
		AssertIdx:     0,
	}

	// Snapshot only the spec subtree.
	spec := &dsl.MatchSnapshotSpec{Path: "spec"}
	r := evalMatchSnapshot(spec, ctx)
	if !r.Pass {
		t.Fatalf("first path snapshot should pass: %s", r.Message)
	}

	// Same doc — should match.
	r = evalMatchSnapshot(spec, ctx)
	if !r.Pass {
		t.Fatalf("second path snapshot should match: %s", r.Message)
	}
}

func TestEvalMatchSnapshot_PathNotFound(t *testing.T) {
	doc := map[string]any{"kind": "Deployment"}
	store := newStore(t, false)
	ctx := EvalContext{
		Doc:           doc,
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "test",
		AssertIdx:     0,
	}

	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{Path: "missing.path"}, ctx)
	if r.Pass {
		t.Error("expected fail for missing path")
	}
}

func TestEvalMatchSnapshot_NilDocWithPath(t *testing.T) {
	store := newStore(t, false)
	ctx := EvalContext{
		Doc:           nil,
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "test",
		AssertIdx:     0,
	}

	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{Path: "spec"}, ctx)
	if r.Pass {
		t.Error("expected fail: path-based snapshot requires a doc")
	}
}

func TestEvalMatchSnapshot_HelperOutput(t *testing.T) {
	store := newStore(t, false)
	ctx := EvalContext{
		IsHelperTest:  true,
		HelperOutput:  "myrepo/myapp:v1.0",
		SnapshotStore: store,
		SuiteName:     "helpers",
		TestName:      "image-string",
		AssertIdx:     0,
	}

	// No path in spec — uses HelperOutput.
	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("first helper snapshot should pass: %s", r.Message)
	}

	r = evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("second helper snapshot should match: %s", r.Message)
	}
}
