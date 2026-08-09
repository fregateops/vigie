package matchers

import (
	"fmt"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "exists",
		matches:  func(a dsl.Assertion) bool { return a.Exists != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalExists(a.Exists, ctx) },
	})
	Register(simpleMatcher{
		name:     "notExists",
		matches:  func(a dsl.Assertion) bool { return a.NotExists != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalNotExists(a.NotExists, ctx) },
	})
}

func evalExists(spec *dsl.PathOnly, ctx EvalContext) Result {
	if ctx.Doc == nil {
		return Result{Pass: false, Message: "no document selected"}
	}
	_, found, err := resolvePathInDoc(ctx.Doc, spec.Path)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("path error: %v", err)}
	}
	if !found {
		return Result{Pass: false, Message: fmt.Sprintf("exists: path %q not found", spec.Path)}
	}
	return Result{Pass: true}
}

func evalNotExists(spec *dsl.PathOnly, ctx EvalContext) Result {
	if ctx.Doc == nil {
		return Result{Pass: false, Message: "no document selected"}
	}
	_, found, err := resolvePathInDoc(ctx.Doc, spec.Path)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("path error: %v", err)}
	}
	if found {
		return Result{Pass: false, Message: fmt.Sprintf("notExists: path %q exists but should not", spec.Path)}
	}
	return Result{Pass: true}
}
