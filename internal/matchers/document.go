package matchers

import (
	"fmt"
	"regexp"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "isKind",
		matches:  func(a dsl.Assertion) bool { return a.IsKind != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsKind(a.IsKind, ctx) },
	})
	Register(simpleMatcher{
		name:     "isAPIVersion",
		matches:  func(a dsl.Assertion) bool { return a.IsAPIVersion != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsAPIVersion(a.IsAPIVersion, ctx) },
	})
	Register(simpleMatcher{
		name:     "hasDocuments",
		matches:  func(a dsl.Assertion) bool { return a.HasDocuments != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalHasDocuments(a.HasDocuments, ctx) },
	})
	Register(simpleMatcher{
		name:     "failedTemplate",
		matches:  func(a dsl.Assertion) bool { return a.FailedTemplate != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalFailedTemplate(a.FailedTemplate, ctx) },
	})
}

func evalIsKind(want *string, ctx EvalContext) Result {
	if ctx.Doc == nil {
		return Result{Pass: false, Message: "no document selected"}
	}
	kind, _, _ := resolvePathInDoc(ctx.Doc, "kind")
	if kind == *want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isKind: expected %q, got %v", *want, kind),
	}
}

func evalIsAPIVersion(want *string, ctx EvalContext) Result {
	if ctx.Doc == nil {
		return Result{Pass: false, Message: "no document selected"}
	}
	apiVersion, _, _ := resolvePathInDoc(ctx.Doc, "apiVersion")
	if apiVersion == *want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isAPIVersion: expected %q, got %v", *want, apiVersion),
	}
}

func evalHasDocuments(want *int, ctx EvalContext) Result {
	n := len(ctx.Docs)
	if n == *want {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("hasDocuments: expected %d, got %d", *want, n),
	}
}

func evalFailedTemplate(spec *dsl.FailedTemplateSpec, ctx EvalContext) Result {
	if ctx.RenderErr == nil {
		return Result{Pass: false, Message: "failedTemplate: render succeeded but expected failure"}
	}
	if spec.ErrorPattern != "" {
		re, err := regexp.Compile(spec.ErrorPattern)
		if err != nil {
			return Result{Pass: false, Message: fmt.Sprintf("failedTemplate: invalid errorPattern regex: %v", err)}
		}
		if !re.MatchString(ctx.RenderErr.Error()) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("failedTemplate: error %q does not match pattern %q", ctx.RenderErr.Error(), spec.ErrorPattern),
			}
		}
	}
	return Result{Pass: true}
}
