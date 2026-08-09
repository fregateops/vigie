package matchers

import (
	"fmt"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "equal",
		matches:  func(a dsl.Assertion) bool { return a.Equal != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalEqual(a.Equal, ctx) },
	})
	Register(simpleMatcher{
		name:     "notEqual",
		matches:  func(a dsl.Assertion) bool { return a.NotEqual != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalNotEqual(a.NotEqual, ctx) },
	})
}

func evalEqual(spec *dsl.PathValue, ctx EvalContext) Result {
	got, _, err := resolveDocValue(ctx, spec.Path, "equal")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if !deepEqual(got, spec.Value) {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("equal: path %q\n          expected: %v\n          got:      %v", spec.Path, spec.Value, fmt.Sprintf("%v", got)),
		}
	}
	return Result{Pass: true}
}

func evalNotEqual(spec *dsl.PathValue, ctx EvalContext) Result {
	if ctx.Doc == nil && !ctx.IsHelperTest {
		return Result{Pass: false, Message: "no document selected"}
	}
	if !ctx.IsHelperTest {
		got, found, err := resolvePathInDoc(ctx.Doc, spec.Path)
		if err != nil {
			return Result{Pass: false, Message: fmt.Sprintf("path error: %v", err)}
		}
		if !found {
			return Result{Pass: true}
		}
		if deepEqual(got, spec.Value) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("notEqual: path %q\n          expected not: %v\n          got:          %v", spec.Path, spec.Value, fmt.Sprintf("%v", got)),
			}
		}
		return Result{Pass: true}
	}
	// Helper test path.
	got, _, err := resolveDocValue(ctx, spec.Path, "notEqual")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if deepEqual(got, spec.Value) {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("notEqual: path %q\n          expected not: %v\n          got:          %v", spec.Path, spec.Value, fmt.Sprintf("%v", got)),
		}
	}
	return Result{Pass: true}
}
