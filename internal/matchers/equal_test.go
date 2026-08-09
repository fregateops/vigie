package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestEvalEqual(t *testing.T) {
	doc := map[string]any{
		"kind": "Deployment",
		"spec": map[string]any{
			"replicas": 3,
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "app", "image": "myrepo/app:v1"},
					},
				},
			},
		},
	}

	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{
			name: "string match",
			spec: dsl.PathValue{Path: "kind", Value: "Deployment"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "int match — same type",
			spec: dsl.PathValue{Path: "spec.replicas", Value: 3},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "int match — int vs float64 (YAML numeric coercion)",
			spec: dsl.PathValue{Path: "spec.replicas", Value: float64(3)},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "nested path match",
			spec: dsl.PathValue{Path: "spec.template.spec.containers[0].image", Value: "myrepo/app:v1"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "value mismatch",
			spec: dsl.PathValue{Path: "kind", Value: "Service"},
			ctx:  EvalContext{Doc: doc},
			pass: false,
		},
		{
			name: "path not found",
			spec: dsl.PathValue{Path: "spec.missing", Value: "x"},
			ctx:  EvalContext{Doc: doc},
			pass: false,
		},
		{
			name: "nil doc",
			spec: dsl.PathValue{Path: "kind", Value: "Deployment"},
			ctx:  EvalContext{Doc: nil},
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalEqual(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
			if !r.Pass && r.Message == "" {
				t.Error("expected non-empty failure message")
			}
		})
	}
}

func TestEvalNotEqual(t *testing.T) {
	doc := map[string]any{"kind": "Deployment", "spec": map[string]any{"replicas": 3}}

	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{
			name: "values differ — pass",
			spec: dsl.PathValue{Path: "kind", Value: "Service"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "values equal — fail",
			spec: dsl.PathValue{Path: "kind", Value: "Deployment"},
			ctx:  EvalContext{Doc: doc},
			pass: false,
		},
		{
			name: "path missing — pass (nothing to compare)",
			spec: dsl.PathValue{Path: "missing", Value: "x"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "nil doc",
			spec: dsl.PathValue{Path: "kind", Value: "x"},
			ctx:  EvalContext{Doc: nil},
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalNotEqual(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestDeepEqual_NumericCoercion(t *testing.T) {
	cases := []struct {
		a, b any
		want bool
	}{
		{int(3), int(3), true},
		{int(3), float64(3), true},
		{float64(3), int64(3), true},
		{int32(3), float32(3), true},
		{int(3), int(4), false},
		{"3", 3, false},
		{nil, nil, true},
	}
	for _, c := range cases {
		got := deepEqual(c.a, c.b)
		if got != c.want {
			t.Errorf("deepEqual(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
