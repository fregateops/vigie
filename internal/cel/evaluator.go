package cel

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// Eval evaluates a CEL expression with the given bindings.
// All bindings are declared as cel.DynType.
// Returns the Go-native value (string, int64, float64, bool, map, slice, etc.)
func Eval(expr string, bindings map[string]any) (any, error) {
	// Build variable declarations from bindings keys.
	opts := make([]cel.EnvOption, 0, len(bindings))
	for k := range bindings {
		opts = append(opts, cel.Variable(k, cel.DynType))
	}

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("cel: create env: %w", err)
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("cel: compile %q: %w", expr, issues.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel: create program: %w", err)
	}

	out, _, err := prg.Eval(bindings)
	if err != nil {
		return nil, fmt.Errorf("cel: eval %q: %w", expr, err)
	}

	return out.Value(), nil
}
