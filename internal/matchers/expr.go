package matchers

import (
	"fmt"

	celpkg "github.com/fregateops/vigie/internal/cel"
	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "expr",
		matches:  func(a dsl.Assertion) bool { return a.Expr != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalExpr(a.Expr, ctx) },
	})
}

// evalExpr evaluates a CEL expression against the eval context.
// The expression must evaluate to a bool; true = pass.
func evalExpr(spec *string, ctx EvalContext) Result {
	// Build the resources map: Kind -> []map[string]any
	resources := make(map[string]any)
	for _, doc := range ctx.Docs {
		kind := doc["kind"]
		kindStr, ok := kind.(string)
		if !ok {
			kindStr = fmt.Sprintf("%v", kind)
		}
		existing := resources[kindStr]
		if existing == nil {
			resources[kindStr] = []any{doc}
		} else {
			resources[kindStr] = append(existing.([]any), doc)
		}
	}

	bindings := map[string]any{
		"resources": resources,
		"doc":       ctx.Doc,
		"release":   map[string]any{},
		"values":    map[string]any{},
		"matrix":    ctx.MatrixEntry,
		"case":      ctx.CaseEntry,
		"output":    ctx.HelperOutput,
	}

	result, err := celpkg.Eval(*spec, bindings)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("expr: evaluation error: %v", err)}
	}

	b, ok := result.(bool)
	if !ok {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("expr: expression %q did not return bool (got %T: %v)", *spec, result, result),
		}
	}
	if b {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("expr: expression %q evaluated to false", *spec),
	}
}
