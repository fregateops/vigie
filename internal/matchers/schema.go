package matchers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/kaptinlin/jsonschema"
)

func init() {
	Register(simpleMatcher{
		name:     "matchSchema",
		matches:  func(a dsl.Assertion) bool { return a.MatchSchema != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalMatchSchema(a.MatchSchema, ctx) },
	})
}

func evalMatchSchema(spec *dsl.MatchSchemaSpec, ctx EvalContext) Result {
	val, _, err := resolveDocValue(ctx, spec.Path, "matchSchema")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}

	// Compile the inline schema.
	schemaBytes, err := json.Marshal(normalizeForJSON(spec.Schema))
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("matchSchema: marshaling schema: %v", err)}
	}
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("matchSchema: compiling schema: %v", err)}
	}

	// Marshal the value to JSON for validation.
	valBytes, err := json.Marshal(normalizeForJSON(val))
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("matchSchema: marshaling value: %v", err)}
	}

	result := compiled.ValidateJSON(valBytes)
	if result.IsValid() {
		return Result{Pass: true}
	}

	var msgs []string
	for _, e := range result.Errors {
		msgs = append(msgs, e.Error())
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("matchSchema: %s", strings.Join(msgs, "; ")),
	}
}

// normalizeForJSON ensures all map keys are strings for JSON marshaling.
func normalizeForJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v2 := range val {
			out[k] = normalizeForJSON(v2)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v2 := range val {
			out[i] = normalizeForJSON(v2)
		}
		return out
	default:
		return val
	}
}
