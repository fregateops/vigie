package cel

import (
	"testing"
)

func TestEval_Arithmetic(t *testing.T) {
	result, err := Eval("1 + 1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := result.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", result, result)
	}
	if v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
}

func TestEval_Comparison_True(t *testing.T) {
	result, err := Eval("x > 5", map[string]any{"x": 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := result.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T: %v", result, result)
	}
	if !b {
		t.Error("expected true for x=10 > 5")
	}
}

func TestEval_Comparison_False(t *testing.T) {
	result, err := Eval("x > 5", map[string]any{"x": 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := result.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T: %v", result, result)
	}
	if b {
		t.Error("expected false for x=3 > 5")
	}
}

func TestEval_StringConcatenation(t *testing.T) {
	result, err := Eval("greeting + ' world'", map[string]any{"greeting": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", result, result)
	}
	if s != "hello world" {
		t.Errorf("expected 'hello world', got %q", s)
	}
}

func TestEval_NestedMapAccess(t *testing.T) {
	result, err := Eval("m.tier == 'small'", map[string]any{
		"m": map[string]any{"tier": "small"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := result.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T: %v", result, result)
	}
	if !b {
		t.Error("expected true for m.tier == 'small'")
	}
}

func TestEval_EmptyBindings(t *testing.T) {
	// Expressions that don't reference any bindings should still work.
	result, err := Eval("'hello' + '!'", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello!" {
		t.Errorf("expected 'hello!', got %v", result)
	}
}

func TestEval_BooleanLogic(t *testing.T) {
	cases := []struct {
		name     string
		expr     string
		bindings map[string]any
		want     bool
	}{
		{"and true", "a && b", map[string]any{"a": true, "b": true}, true},
		{"and false", "a && b", map[string]any{"a": true, "b": false}, false},
		{"or true", "a || b", map[string]any{"a": false, "b": true}, true},
		{"or false", "a || b", map[string]any{"a": false, "b": false}, false},
		{"not", "!a", map[string]any{"a": false}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Eval(tc.expr, tc.bindings)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			b, ok := result.(bool)
			if !ok {
				t.Fatalf("expected bool, got %T", result)
			}
			if b != tc.want {
				t.Errorf("got %v, want %v", b, tc.want)
			}
		})
	}
}

func TestEval_InvalidExpression_CompileError(t *testing.T) {
	_, err := Eval("@@not valid cel@@", nil)
	if err == nil {
		t.Error("expected an error for invalid CEL expression, got nil")
	}
}

func TestEval_UndeclaredVariable(t *testing.T) {
	// Referencing a variable not in bindings should produce a compile/eval error.
	_, err := Eval("undeclared > 0", nil)
	if err == nil {
		t.Error("expected an error for undeclared variable, got nil")
	}
}

func TestEval_TernaryExpression(t *testing.T) {
	result, err := Eval("x > 0 ? 'positive' : 'non-positive'", map[string]any{"x": 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "positive" {
		t.Errorf("expected 'positive', got %v", result)
	}
}
