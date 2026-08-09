package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

var typeDoc = map[string]any{
	"str":     "hello",
	"num":     42,
	"flt":     3.14,
	"boolean": true,
	"list":    []any{"a", "b"},
	"mapping": map[string]any{"k": "v"},
	"nullval": nil,
	"empty":   "",
}

// ── isNull / isNotNull ──────────────────────────────────────────────────────

func TestEvalIsNull(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathOnly
		ctx  EvalContext
		pass bool
	}{
		{"null value — pass", dsl.PathOnly{Path: "nullval"}, EvalContext{Doc: typeDoc}, true},
		{"non-null string — fail", dsl.PathOnly{Path: "str"}, EvalContext{Doc: typeDoc}, false},
		{"non-null int — fail", dsl.PathOnly{Path: "num"}, EvalContext{Doc: typeDoc}, false},
		{"path not found fails", dsl.PathOnly{Path: "missing"}, EvalContext{Doc: typeDoc}, false},
		{"nil doc fails", dsl.PathOnly{Path: "nullval"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsNull(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalIsNotNull(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathOnly
		ctx  EvalContext
		pass bool
	}{
		{"non-null string — pass", dsl.PathOnly{Path: "str"}, EvalContext{Doc: typeDoc}, true},
		{"non-null int — pass", dsl.PathOnly{Path: "num"}, EvalContext{Doc: typeDoc}, true},
		{"null value — fail", dsl.PathOnly{Path: "nullval"}, EvalContext{Doc: typeDoc}, false},
		{"path not found fails", dsl.PathOnly{Path: "missing"}, EvalContext{Doc: typeDoc}, false},
		{"nil doc fails", dsl.PathOnly{Path: "str"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsNotNull(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

// ── isEmpty / isNotEmpty ────────────────────────────────────────────────────

func TestEvalIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathOnly
		ctx  EvalContext
		pass bool
	}{
		{"null is empty", dsl.PathOnly{Path: "nullval"}, EvalContext{Doc: typeDoc}, true},
		{"empty string is empty", dsl.PathOnly{Path: "empty"}, EvalContext{Doc: typeDoc}, true},
		{"non-empty string — fail", dsl.PathOnly{Path: "str"}, EvalContext{Doc: typeDoc}, false},
		{"non-empty list — fail", dsl.PathOnly{Path: "list"}, EvalContext{Doc: typeDoc}, false},
		{"non-empty map — fail", dsl.PathOnly{Path: "mapping"}, EvalContext{Doc: typeDoc}, false},
		{"path not found fails", dsl.PathOnly{Path: "missing"}, EvalContext{Doc: typeDoc}, false},
		{"nil doc fails", dsl.PathOnly{Path: "empty"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsEmpty(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalIsNotEmpty(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathOnly
		ctx  EvalContext
		pass bool
	}{
		{"non-empty string — pass", dsl.PathOnly{Path: "str"}, EvalContext{Doc: typeDoc}, true},
		{"non-empty list — pass", dsl.PathOnly{Path: "list"}, EvalContext{Doc: typeDoc}, true},
		{"non-empty map — pass", dsl.PathOnly{Path: "mapping"}, EvalContext{Doc: typeDoc}, true},
		{"null is empty — fail", dsl.PathOnly{Path: "nullval"}, EvalContext{Doc: typeDoc}, false},
		{"empty string — fail", dsl.PathOnly{Path: "empty"}, EvalContext{Doc: typeDoc}, false},
		{"path not found fails", dsl.PathOnly{Path: "missing"}, EvalContext{Doc: typeDoc}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsNotEmpty(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

// ── isType ──────────────────────────────────────────────────────────────────

func TestEvalIsType(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.IsTypeSpec
		ctx  EvalContext
		pass bool
	}{
		{"string", dsl.IsTypeSpec{Path: "str", Of: "string"}, EvalContext{Doc: typeDoc}, true},
		{"int", dsl.IsTypeSpec{Path: "num", Of: "int"}, EvalContext{Doc: typeDoc}, true},
		{"float64 as float", dsl.IsTypeSpec{Path: "flt", Of: "float"}, EvalContext{Doc: typeDoc}, true},
		{"whole float64 as int", dsl.IsTypeSpec{Path: "flt", Of: "int"}, EvalContext{Doc: typeDoc}, false},
		{"bool", dsl.IsTypeSpec{Path: "boolean", Of: "bool"}, EvalContext{Doc: typeDoc}, true},
		{"list", dsl.IsTypeSpec{Path: "list", Of: "list"}, EvalContext{Doc: typeDoc}, true},
		{"map", dsl.IsTypeSpec{Path: "mapping", Of: "map"}, EvalContext{Doc: typeDoc}, true},
		{"wrong type string vs int", dsl.IsTypeSpec{Path: "str", Of: "int"}, EvalContext{Doc: typeDoc}, false},
		{"wrong type int vs bool", dsl.IsTypeSpec{Path: "num", Of: "bool"}, EvalContext{Doc: typeDoc}, false},
		{"null value fails any type", dsl.IsTypeSpec{Path: "nullval", Of: "string"}, EvalContext{Doc: typeDoc}, false},
		// Whole-number float64 (YAML often decodes integers as float64)
		{
			"float64 whole number passes int check",
			dsl.IsTypeSpec{Path: "wholeFloat", Of: "int"},
			EvalContext{Doc: map[string]any{"wholeFloat": float64(3)}},
			true,
		},
		{"path not found fails", dsl.IsTypeSpec{Path: "missing", Of: "string"}, EvalContext{Doc: typeDoc}, false},
		{"nil doc fails", dsl.IsTypeSpec{Path: "str", Of: "string"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsType(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

// ── lengthEqual ─────────────────────────────────────────────────────────────

func TestEvalLengthEqual(t *testing.T) {
	doc := map[string]any{
		"word":    "hello",
		"items":   []any{"a", "b", "c"},
		"mapping": map[string]any{"x": 1, "y": 2},
		"num":     42,
	}
	tests := []struct {
		name string
		spec dsl.LengthEqualSpec
		ctx  EvalContext
		pass bool
	}{
		{"string length match", dsl.LengthEqualSpec{Path: "word", Value: 5}, EvalContext{Doc: doc}, true},
		{"string length mismatch", dsl.LengthEqualSpec{Path: "word", Value: 3}, EvalContext{Doc: doc}, false},
		{"slice length match", dsl.LengthEqualSpec{Path: "items", Value: 3}, EvalContext{Doc: doc}, true},
		{"slice length mismatch", dsl.LengthEqualSpec{Path: "items", Value: 2}, EvalContext{Doc: doc}, false},
		{"map length match", dsl.LengthEqualSpec{Path: "mapping", Value: 2}, EvalContext{Doc: doc}, true},
		{"map length mismatch", dsl.LengthEqualSpec{Path: "mapping", Value: 1}, EvalContext{Doc: doc}, false},
		{"non-measurable type fails", dsl.LengthEqualSpec{Path: "num", Value: 2}, EvalContext{Doc: doc}, false},
		{"path not found fails", dsl.LengthEqualSpec{Path: "missing", Value: 0}, EvalContext{Doc: doc}, false},
		{"nil doc fails", dsl.LengthEqualSpec{Path: "word", Value: 5}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalLengthEqual(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

// ── isSubset ────────────────────────────────────────────────────────────────

func TestEvalIsSubset(t *testing.T) {
	doc := map[string]any{
		"labels": map[string]any{
			"app":     "my-app",
			"version": "v1.0",
			"env":     "prod",
		},
		"name": "my-release",
	}
	tests := []struct {
		name string
		spec dsl.PathContent
		ctx  EvalContext
		pass bool
	}{
		{
			"single-key subset — pass",
			dsl.PathContent{Path: "labels", Content: map[string]any{"app": "my-app"}},
			EvalContext{Doc: doc},
			true,
		},
		{
			"multi-key subset — pass",
			dsl.PathContent{Path: "labels", Content: map[string]any{"app": "my-app", "env": "prod"}},
			EvalContext{Doc: doc},
			true,
		},
		{
			"exact match — pass",
			dsl.PathContent{Path: "labels", Content: map[string]any{"app": "my-app", "version": "v1.0", "env": "prod"}},
			EvalContext{Doc: doc},
			true,
		},
		{
			"wrong value — fail",
			dsl.PathContent{Path: "labels", Content: map[string]any{"app": "wrong-app"}},
			EvalContext{Doc: doc},
			false,
		},
		{
			"missing key in target — fail",
			dsl.PathContent{Path: "labels", Content: map[string]any{"missing-key": "x"}},
			EvalContext{Doc: doc},
			false,
		},
		{
			"target is not a map — fail",
			dsl.PathContent{Path: "name", Content: map[string]any{"k": "v"}},
			EvalContext{Doc: doc},
			false,
		},
		{
			"content is not a map — fail",
			dsl.PathContent{Path: "labels", Content: "not-a-map"},
			EvalContext{Doc: doc},
			false,
		},
		{"path not found fails", dsl.PathContent{Path: "missing", Content: map[string]any{}}, EvalContext{Doc: doc}, false},
		{"nil doc fails", dsl.PathContent{Path: "labels", Content: map[string]any{}}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsSubset(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}
