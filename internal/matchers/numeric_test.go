package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

var numericDoc = map[string]any{
	"replicas": 3,
	"weight":   float64(1.5),
	"label":    "text",
}

func TestEvalGreaterThan(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{"greater than — pass", dsl.PathValue{Path: "replicas", Value: 2}, EvalContext{Doc: numericDoc}, true},
		{"equal — fail", dsl.PathValue{Path: "replicas", Value: 3}, EvalContext{Doc: numericDoc}, false},
		{"less than — fail", dsl.PathValue{Path: "replicas", Value: 5}, EvalContext{Doc: numericDoc}, false},
		{"float field greater — pass", dsl.PathValue{Path: "weight", Value: 1.0}, EvalContext{Doc: numericDoc}, true},
		{"float field equal — fail", dsl.PathValue{Path: "weight", Value: 1.5}, EvalContext{Doc: numericDoc}, false},
		{"non-numeric field fails", dsl.PathValue{Path: "label", Value: 0}, EvalContext{Doc: numericDoc}, false},
		{"non-numeric spec value fails", dsl.PathValue{Path: "replicas", Value: "not-a-number"}, EvalContext{Doc: numericDoc}, false},
		{"path not found fails", dsl.PathValue{Path: "missing", Value: 0}, EvalContext{Doc: numericDoc}, false},
		{"nil doc fails", dsl.PathValue{Path: "replicas", Value: 0}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalGreaterThan(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalLessThan(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{"less than — pass", dsl.PathValue{Path: "replicas", Value: 5}, EvalContext{Doc: numericDoc}, true},
		{"equal — fail", dsl.PathValue{Path: "replicas", Value: 3}, EvalContext{Doc: numericDoc}, false},
		{"greater than — fail", dsl.PathValue{Path: "replicas", Value: 2}, EvalContext{Doc: numericDoc}, false},
		{"path not found fails", dsl.PathValue{Path: "missing", Value: 10}, EvalContext{Doc: numericDoc}, false},
		{"nil doc fails", dsl.PathValue{Path: "replicas", Value: 10}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalLessThan(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalGTE(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{"greater — pass", dsl.PathValue{Path: "replicas", Value: 2}, EvalContext{Doc: numericDoc}, true},
		{"equal — pass", dsl.PathValue{Path: "replicas", Value: 3}, EvalContext{Doc: numericDoc}, true},
		{"less — fail", dsl.PathValue{Path: "replicas", Value: 4}, EvalContext{Doc: numericDoc}, false},
		{"path not found fails", dsl.PathValue{Path: "missing", Value: 0}, EvalContext{Doc: numericDoc}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalGTE(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalLTE(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{"less — pass", dsl.PathValue{Path: "replicas", Value: 4}, EvalContext{Doc: numericDoc}, true},
		{"equal — pass", dsl.PathValue{Path: "replicas", Value: 3}, EvalContext{Doc: numericDoc}, true},
		{"greater — fail", dsl.PathValue{Path: "replicas", Value: 2}, EvalContext{Doc: numericDoc}, false},
		{"path not found fails", dsl.PathValue{Path: "missing", Value: 10}, EvalContext{Doc: numericDoc}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalLTE(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}
