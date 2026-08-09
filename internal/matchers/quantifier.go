package matchers

import (
	"fmt"
	"strings"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(allOfMatcher{})
	Register(anyOfMatcher{})
}

type allOfMatcher struct{}

func (allOfMatcher) Name() string                 { return "allOf" }
func (allOfMatcher) Matches(a dsl.Assertion) bool { return len(a.AllOf) > 0 }
func (allOfMatcher) SupportedTiers(a dsl.Assertion) []string {
	return intersectChildTiers(a.AllOf)
}
func (allOfMatcher) Evaluate(a dsl.Assertion, ctx EvalContext) Result {
	return evalAllOf(a.AllOf, ctx)
}

// evalAllOf evaluates each child assertion and requires all to pass.
func evalAllOf(specs []dsl.Assertion, ctx EvalContext) Result {
	var failures []string
	for i, child := range specs {
		r := Evaluate(child, ctx)
		if !r.Pass {
			failures = append(failures, fmt.Sprintf("  [%d] %s", i, r.Message))
		}
	}
	if len(failures) == 0 {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("allOf: %d of %d assertions failed:\n%s", len(failures), len(specs), strings.Join(failures, "\n")),
	}
}

// anyOfMatcher uses intersection (same as allOf) rather than union. The runner
// can't know in advance which branch will pass, so it must satisfy the
// strictest child to avoid silently bypassing the high-tier assertion the
// user added on purpose.
type anyOfMatcher struct{}

func (anyOfMatcher) Name() string                 { return "anyOf" }
func (anyOfMatcher) Matches(a dsl.Assertion) bool { return len(a.AnyOf) > 0 }
func (anyOfMatcher) SupportedTiers(a dsl.Assertion) []string {
	return intersectChildTiers(a.AnyOf)
}
func (anyOfMatcher) Evaluate(a dsl.Assertion, ctx EvalContext) Result {
	return evalAnyOf(a.AnyOf, ctx)
}

// evalAnyOf evaluates each child assertion and requires at least one to pass.
func evalAnyOf(specs []dsl.Assertion, ctx EvalContext) Result {
	var messages []string
	for i, child := range specs {
		r := Evaluate(child, ctx)
		if r.Pass {
			return Result{Pass: true}
		}
		messages = append(messages, fmt.Sprintf("  [%d] %s", i, r.Message))
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("anyOf: none of %d assertions passed:\n%s", len(specs), strings.Join(messages, "\n")),
	}
}
