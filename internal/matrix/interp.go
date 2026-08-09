package matrix

import (
	"fmt"
	"regexp"
	"strings"

	celeval "github.com/fregateops/vigie/internal/cel"
)

var interpRe = regexp.MustCompile(`\$\{\{(.*?)\}\}`)

// Interpolate walks a value (map, slice, string, etc.) and replaces
// any string of the form "${{ expr }}" with the result of evaluating
// the CEL expression with the given bindings.
//
// Two modes:
//  1. Whole string is "${{ expr }}" → return the typed CEL result (may be bool, int, string, etc.)
//  2. String contains "${{ expr }}" as substring → do string replacement (all occurrences → string repr)
//
// Non-string values pass through unchanged.
// Maps and slices are recursively walked.
func Interpolate(v any, bindings map[string]any) (any, error) {
	switch val := v.(type) {
	case string:
		return interpolateString(val, bindings)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			replaced, err := Interpolate(elem, bindings)
			if err != nil {
				return nil, err
			}
			out[k] = replaced
		}
		return out, nil
	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			replaced, err := Interpolate(elem, bindings)
			if err != nil {
				return nil, err
			}
			out[i] = replaced
		}
		return out, nil
	default:
		return v, nil
	}
}

func interpolateString(s string, bindings map[string]any) (any, error) {
	matches := interpRe.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return s, nil
	}

	// Mode 1: the entire string is a single "${{ expr }}" — return typed result.
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) {
		sub := interpRe.FindStringSubmatch(s)
		expr := strings.TrimSpace(sub[1])
		result, err := celeval.Eval(expr, bindings)
		if err != nil {
			return nil, fmt.Errorf("interpolate: %w", err)
		}
		return result, nil
	}

	// Mode 2: one or more occurrences embedded in a larger string — replace with string repr.
	out := interpRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := interpRe.FindStringSubmatch(match)
		expr := strings.TrimSpace(sub[1])
		result, err := celeval.Eval(expr, bindings)
		if err != nil {
			// Embed error text so caller can detect it if needed; the error
			// path returns properly below via a second pass when we re-check.
			return fmt.Sprintf("%%!(CEL_ERROR:%v)", err)
		}
		return fmt.Sprintf("%v", result)
	})

	// Check whether any replacement produced an error sentinel.
	if strings.Contains(out, "%(CEL_ERROR:") || strings.Contains(out, "%!(CEL_ERROR:") {
		// Re-evaluate to surface the real error.
		var retErr error
		interpRe.ReplaceAllStringFunc(s, func(match string) string {
			sub := interpRe.FindStringSubmatch(match)
			expr := strings.TrimSpace(sub[1])
			_, err := celeval.Eval(expr, bindings)
			if err != nil && retErr == nil {
				retErr = fmt.Errorf("interpolate: %w", err)
			}
			return ""
		})
		if retErr != nil {
			return nil, retErr
		}
	}

	return out, nil
}
