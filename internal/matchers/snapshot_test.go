package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/snapshot"
)

func newStore(t *testing.T) *snapshot.Store {
	t.Helper()
	return &snapshot.Store{Dir: t.TempDir()}
}

// record writes the snapshot for ctx the way `vigie test -u` does, leaving the
// store back in compare mode.
func record(t *testing.T, spec *dsl.MatchSnapshotSpec, ctx EvalContext) {
	t.Helper()
	ctx.SnapshotStore.Mode = snapshot.Update
	defer func() { ctx.SnapshotStore.Mode = snapshot.Compare }()
	if r := evalMatchSnapshot(spec, ctx); !r.Pass {
		t.Fatalf("recording snapshot: %s", r.Message)
	}
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
	store := newStore(t)
	ctx := EvalContext{
		Doc:           doc,
		SnapshotStore: store,
		SuiteName:     "my-suite",
		TestName:      "my-test",
		AssertIdx:     0,
	}

	record(t, &dsl.MatchSnapshotSpec{}, ctx)
	if r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx); !r.Pass {
		t.Fatalf("recorded snapshot should match: %s", r.Message)
	}
}

func TestEvalMatchSnapshot_Mismatch(t *testing.T) {
	store := newStore(t)
	ctx := EvalContext{
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "test",
		AssertIdx:     0,
	}

	ctx.Doc = map[string]any{"kind": "Deployment"}
	record(t, &dsl.MatchSnapshotSpec{}, ctx)

	ctx.Doc = map[string]any{"kind": "Service"}
	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
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
	store := newStore(t)
	ctx := EvalContext{
		Doc:           doc1,
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "test",
		AssertIdx:     0,
	}

	record(t, &dsl.MatchSnapshotSpec{}, ctx)

	ctx.Doc = doc2
	if r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx); r.Pass {
		t.Fatal("compare mode must not accept a changed value")
	}
	record(t, &dsl.MatchSnapshotSpec{}, ctx)

	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("after update, doc2 should match: %s", r.Message)
	}
}

func TestEvalMatchSnapshot_PathBased(t *testing.T) {
	doc := map[string]any{
		"kind": "Deployment",
		"spec": map[string]any{"replicas": 3},
	}
	store := newStore(t)
	ctx := EvalContext{
		Doc:           doc,
		SnapshotStore: store,
		SuiteName:     "suite",
		TestName:      "path-test",
		AssertIdx:     0,
	}

	spec := &dsl.MatchSnapshotSpec{Path: "spec"}
	record(t, spec, ctx)

	r := evalMatchSnapshot(spec, ctx)
	if !r.Pass {
		t.Fatalf("second path snapshot should match: %s", r.Message)
	}
}

func TestEvalMatchSnapshot_PathNotFound(t *testing.T) {
	doc := map[string]any{"kind": "Deployment"}
	store := newStore(t)
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
	store := newStore(t)
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
	store := newStore(t)
	ctx := EvalContext{
		IsHelperTest:  true,
		HelperOutput:  "myrepo/myapp:v1.0",
		SnapshotStore: store,
		SuiteName:     "helpers",
		TestName:      "image-string",
		AssertIdx:     0,
	}

	record(t, &dsl.MatchSnapshotSpec{}, ctx)

	r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx)
	if !r.Pass {
		t.Fatalf("second helper snapshot should match: %s", r.Message)
	}
}

func TestEvalMatchSnapshot_MissingFailsAndIsNotCreated(t *testing.T) {
	ctx := EvalContext{
		Doc:           map[string]any{"kind": "Deployment"},
		SnapshotStore: newStore(t),
		SuiteName:     "suite",
		TestName:      "test",
	}
	for range 2 {
		if r := evalMatchSnapshot(&dsl.MatchSnapshotSpec{}, ctx); r.Pass {
			t.Fatal("a missing snapshot must fail, and must not be created by the failing run")
		}
	}
}
