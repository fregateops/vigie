package matchers

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/path"
	"github.com/fregateops/vigie/internal/snapshot"
	"k8s.io/client-go/rest"
)

// Result is the outcome of evaluating a single assertion.
type Result struct {
	Pass    bool
	Message string
}

// EvalContext carries the data available to matchers.
type EvalContext struct {
	// Docs is the full set of rendered YAML documents for the test.
	Docs []map[string]any
	// Doc is the target document selected for this assertion (may be nil if none selected).
	Doc map[string]any
	// RenderErr is non-nil when helm rendering failed.
	RenderErr error
	// MatrixEntry is the current matrix combination (populated during matrix expansion).
	MatrixEntry map[string]any
	// CaseEntry is the current case entry (populated during cases expansion).
	CaseEntry map[string]any
	// IsHelperTest is true when the test uses call: (helper template test).
	IsHelperTest bool
	// HelperOutput is the parsed output of a helper template invocation.
	HelperOutput any
	// SnapshotStore is the snapshot store used by the matchSnapshot matcher.
	SnapshotStore *snapshot.Store
	// SuiteName is the name of the test suite (used for snapshot keys).
	SuiteName string
	// TestName is the name of the test (used for snapshot keys).
	TestName string
	// AssertIdx is the index of the assertion within the test (used for snapshot keys).
	AssertIdx int
	// InApplyTier is true when the test runs against a live API server (test-apply api or higher).
	// applies/rejected matchers require this to be true.
	InApplyTier bool
	// ApplyError is non-nil when the helm install rejected the resource; nil means accepted.
	// Only meaningful when InApplyTier is true.
	ApplyError error
	// RESTConfig is the Kubernetes REST client config used by apply-tier matchers.
	// Must be non-nil when InApplyTier is true and live-cluster matchers are used.
	RESTConfig *rest.Config
	// Namespace is the per-test namespace the chart was installed into.
	// Live-cluster matchers (waitFor, becomesReady, lookup, logsContain,
	// eventEmitted, http) fall back to this when their spec doesn't pin one,
	// so tests don't have to know the dynamically-allocated namespace name.
	Namespace string
}

// Evaluate runs a single assertion against ctx and returns its result. The
// concrete matcher is resolved through the registry - each matcher file
// (equal.go, http.go, …) registers its implementation from init(). Negation
// is applied here so individual matchers don't have to care about a.Not.
func Evaluate(a dsl.Assertion, ctx EvalContext) Result {
	var r Result
	if m, ok := find(a); ok {
		r = m.Evaluate(a, ctx)
	} else {
		r = Result{
			Pass:    false,
			Message: fmt.Sprintf("unknown or unsupported matcher (supported: %s)", strings.Join(registeredNames(), ", ")),
		}
	}

	if a.Not {
		r.Pass = !r.Pass
		if r.Pass {
			r.Message = ""
		} else {
			r.Message = "not: " + r.Message
		}
	}
	return r
}

func resolvePathInDoc(doc map[string]any, pathExpr string) (any, bool, error) {
	return path.Get(doc, pathExpr)
}

// resolveDocValue resolves a value for matchers that operate on doc paths.
// In helper test context with an empty path it returns HelperOutput directly.
func resolveDocValue(ctx EvalContext, p string, matcherName string) (any, bool, error) {
	if ctx.IsHelperTest && p == "" {
		return ctx.HelperOutput, true, nil
	}
	if ctx.Doc == nil {
		return nil, false, fmt.Errorf("no document selected")
	}
	val, found, err := resolvePathInDoc(ctx.Doc, p)
	if err != nil {
		return nil, false, fmt.Errorf("path error: %v", err)
	}
	if !found {
		return nil, false, fmt.Errorf("%s: path %q not found", matcherName, p)
	}
	return val, true, nil
}

// resolveNS returns specNS when non-empty, otherwise ctx.Namespace.
// Apply-tier matchers use this so tests can omit an explicit namespace
// and still target the per-test namespace the runner allocated.
func resolveNS(specNS string, ctx EvalContext) string {
	if specNS != "" {
		return specNS
	}
	return ctx.Namespace
}

// deepEqual compares two values, normalizing numeric types for YAML compatibility.
func deepEqual(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	// YAML integers may decode as int vs int64 vs float64 depending on context.
	an := toFloat64(a)
	bn := toFloat64(b)
	if an != nil && bn != nil {
		return *an == *bn
	}
	return false
}

func toFloat64(v any) *float64 {
	switch n := v.(type) {
	case int:
		f := float64(n)
		return &f
	case int32:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case float32:
		f := float64(n)
		return &f
	case float64:
		return &n
	}
	return nil
}
