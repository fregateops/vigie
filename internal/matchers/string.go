package matchers

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "contains",
		matches:  func(a dsl.Assertion) bool { return a.Contains != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalContains(a.Contains, ctx) },
	})
	Register(simpleMatcher{
		name:     "notContains",
		matches:  func(a dsl.Assertion) bool { return a.NotContains != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalNotContains(a.NotContains, ctx) },
	})
	Register(simpleMatcher{
		name:     "startsWith",
		matches:  func(a dsl.Assertion) bool { return a.StartsWith != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalStartsWith(a.StartsWith, ctx) },
	})
	Register(simpleMatcher{
		name:     "endsWith",
		matches:  func(a dsl.Assertion) bool { return a.EndsWith != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalEndsWith(a.EndsWith, ctx) },
	})
	Register(simpleMatcher{
		name:     "matchRegex",
		matches:  func(a dsl.Assertion) bool { return a.MatchRegex != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalMatchRegex(a.MatchRegex, ctx) },
	})
	Register(simpleMatcher{
		name:     "notMatchRegex",
		matches:  func(a dsl.Assertion) bool { return a.NotMatchRegex != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalNotMatchRegex(a.NotMatchRegex, ctx) },
	})
	Register(simpleMatcher{
		name:     "matchTemplate",
		matches:  func(a dsl.Assertion) bool { return a.MatchTemplate != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalMatchTemplate(a.MatchTemplate, ctx) },
	})
}

func evalContains(spec *dsl.PathContent, ctx EvalContext) Result {
	val, err := resolveForString(ctx, spec.Path, "contains")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}

	switch v := val.(type) {
	case string:
		needle, ok := spec.Content.(string)
		if !ok {
			needle = fmt.Sprintf("%v", spec.Content)
		}
		if strings.Contains(v, needle) {
			return Result{Pass: true}
		}
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("contains: path %q: %q does not contain %q", spec.Path, v, needle),
		}
	default:
		// Try slice iteration.
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			for i := 0; i < rv.Len(); i++ {
				if reflect.DeepEqual(rv.Index(i).Interface(), spec.Content) {
					return Result{Pass: true}
				}
			}
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("contains: path %q: slice does not contain %v", spec.Path, spec.Content),
			}
		}
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("contains: path %q: value %v is not a string or slice", spec.Path, val),
		}
	}
}

func evalNotContains(spec *dsl.PathContent, ctx EvalContext) Result {
	// Resolve the path first — a missing path is an error, not a pass.
	val, err := resolveForString(ctx, spec.Path, "notContains")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}

	switch v := val.(type) {
	case string:
		needle, ok := spec.Content.(string)
		if !ok {
			needle = fmt.Sprintf("%v", spec.Content)
		}
		if !strings.Contains(v, needle) {
			return Result{Pass: true}
		}
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("notContains: path %q: %q unexpectedly contains %q", spec.Path, v, needle),
		}
	default:
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			for i := 0; i < rv.Len(); i++ {
				if reflect.DeepEqual(rv.Index(i).Interface(), spec.Content) {
					return Result{
						Pass:    false,
						Message: fmt.Sprintf("notContains: path %q: slice unexpectedly contains %v", spec.Path, spec.Content),
					}
				}
			}
			return Result{Pass: true}
		}
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("notContains: path %q: value %v is not a string or slice", spec.Path, val),
		}
	}
}

func evalStartsWith(spec *dsl.PathValue, ctx EvalContext) Result {
	val, err := resolveForString(ctx, spec.Path, "startsWith")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	str := fmt.Sprintf("%v", val)
	prefix := fmt.Sprintf("%v", spec.Value)
	if strings.HasPrefix(str, prefix) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("startsWith: path %q: %q does not start with %q", spec.Path, str, prefix),
	}
}

func evalEndsWith(spec *dsl.PathValue, ctx EvalContext) Result {
	val, err := resolveForString(ctx, spec.Path, "endsWith")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	str := fmt.Sprintf("%v", val)
	suffix := fmt.Sprintf("%v", spec.Value)
	if strings.HasSuffix(str, suffix) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("endsWith: path %q: %q does not end with %q", spec.Path, str, suffix),
	}
}

func evalMatchRegex(spec *dsl.PathPattern, ctx EvalContext) Result {
	val, err := resolveForString(ctx, spec.Path, "matchRegex")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	re, compErr := regexp.Compile(spec.Pattern)
	if compErr != nil {
		return Result{Pass: false, Message: fmt.Sprintf("matchRegex: invalid pattern %q: %v", spec.Pattern, compErr)}
	}
	str := fmt.Sprintf("%v", val)
	if re.MatchString(str) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("matchRegex: path %q: %q does not match pattern %q", spec.Path, str, spec.Pattern),
	}
}

func evalNotMatchRegex(spec *dsl.PathPattern, ctx EvalContext) Result {
	val, err := resolveForString(ctx, spec.Path, "notMatchRegex")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	re, compErr := regexp.Compile(spec.Pattern)
	if compErr != nil {
		return Result{Pass: false, Message: fmt.Sprintf("notMatchRegex: invalid pattern %q: %v", spec.Pattern, compErr)}
	}
	str := fmt.Sprintf("%v", val)
	if !re.MatchString(str) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("notMatchRegex: path %q: %q matches pattern %q but should not", spec.Path, str, spec.Pattern),
	}
}

// evalMatchTemplate converts a template pattern (with ${VAR} placeholders) into a regex
// and tests the resolved value against it.
func evalMatchTemplate(spec *dsl.PathPattern, ctx EvalContext) Result {
	val, err := resolveForString(ctx, spec.Path, "matchTemplate")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}

	// Replace ${VAR} placeholders with (.+?) and escape the rest.
	placeholderRe := regexp.MustCompile(`\$\{[A-Za-z0-9_]+\}`)
	// Split the pattern on placeholder boundaries, escape the literal parts,
	// then rejoin with (.+?).
	parts := placeholderRe.Split(spec.Pattern, -1)
	escaped := make([]string, len(parts))
	for i, p := range parts {
		escaped[i] = regexp.QuoteMeta(p)
	}
	regexStr := "^" + strings.Join(escaped, "(.+?)") + "$"

	re, compErr := regexp.Compile(regexStr)
	if compErr != nil {
		return Result{Pass: false, Message: fmt.Sprintf("matchTemplate: could not compile derived regex %q: %v", regexStr, compErr)}
	}
	str := fmt.Sprintf("%v", val)
	if re.MatchString(str) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("matchTemplate: path %q: %q does not match template %q", spec.Path, str, spec.Pattern),
	}
}

// resolveForString resolves a path value for string matchers, with a special
// case for helper tests where an empty path returns HelperOutput directly.
func resolveForString(ctx EvalContext, p string, matcherName string) (any, error) {
	if p == "" {
		if ctx.IsHelperTest {
			return ctx.HelperOutput, nil
		}
		return nil, fmt.Errorf("%s: path is required for non-helper tests", matcherName)
	}
	val, _, err := resolveDocValue(ctx, p, matcherName)
	return val, err
}
