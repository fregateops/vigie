package matchers

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/path"
)

// Result is the outcome of evaluating a single assertion.
type Result struct {
	Pass    bool
	Message string
}

// EvalContext carries the data available to matchers.
//
// The snapshot-store fields (matchSnapshot) and the apply-tier fields
// (InApplyTier/ApplyError/RESTConfig/Namespace, used by applies/rejected and
// the live-cluster matchers) are added back when those slices land.
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
}

// Evaluate runs a single assertion against ctx and returns its result. The
// concrete matcher is resolved through the registry — each matcher file
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
