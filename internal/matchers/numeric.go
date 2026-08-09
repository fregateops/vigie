package matchers

import (
	"fmt"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "greaterThan",
		matches:  func(a dsl.Assertion) bool { return a.GreaterThan != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalGreaterThan(a.GreaterThan, ctx) },
	})
	Register(simpleMatcher{
		name:     "lessThan",
		matches:  func(a dsl.Assertion) bool { return a.LessThan != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalLessThan(a.LessThan, ctx) },
	})
	Register(simpleMatcher{
		name:     "gte",
		matches:  func(a dsl.Assertion) bool { return a.GTE != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalGTE(a.GTE, ctx) },
	})
	Register(simpleMatcher{
		name:     "lte",
		matches:  func(a dsl.Assertion) bool { return a.LTE != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalLTE(a.LTE, ctx) },
	})
}

func evalGreaterThan(spec *dsl.PathValue, ctx EvalContext) Result {
	got, want, err := resolveNumericPair(spec, ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if got > want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("greaterThan: path %q: expected %v > %v", spec.Path, got, want),
	}
}

func evalLessThan(spec *dsl.PathValue, ctx EvalContext) Result {
	got, want, err := resolveNumericPair(spec, ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if got < want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("lessThan: path %q: expected %v < %v", spec.Path, got, want),
	}
}

func evalGTE(spec *dsl.PathValue, ctx EvalContext) Result {
	got, want, err := resolveNumericPair(spec, ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if got >= want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("gte: path %q: expected %v >= %v", spec.Path, got, want),
	}
}

func evalLTE(spec *dsl.PathValue, ctx EvalContext) Result {
	got, want, err := resolveNumericPair(spec, ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if got <= want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("lte: path %q: expected %v <= %v", spec.Path, got, want),
	}
}

// resolveNumericPair resolves the path value and spec.Value to float64 for comparison.
func resolveNumericPair(spec *dsl.PathValue, ctx EvalContext) (got float64, want float64, err error) {
	if ctx.Doc == nil {
		return 0, 0, fmt.Errorf("no document selected")
	}
	raw, found, pathErr := resolvePathInDoc(ctx.Doc, spec.Path)
	if pathErr != nil {
		return 0, 0, fmt.Errorf("path error: %v", pathErr)
	}
	if !found {
		return 0, 0, fmt.Errorf("path %q not found", spec.Path)
	}
	gotPtr := toFloat64(raw)
	if gotPtr == nil {
		return 0, 0, fmt.Errorf("path %q: value %v is not numeric", spec.Path, raw)
	}
	wantPtr := toFloat64(spec.Value)
	if wantPtr == nil {
		return 0, 0, fmt.Errorf("expected value %v is not numeric", spec.Value)
	}
	return *gotPtr, *wantPtr, nil
}
