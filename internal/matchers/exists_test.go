package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestEvalExists(t *testing.T) {
	doc := map[string]any{
		"kind": "Deployment",
		"spec": map[string]any{
			"replicas": 3,
			"containers": []any{
				map[string]any{"name": "app"},
			},
		},
		"nullField": nil,
	}

	tests := []struct {
		name string
		spec dsl.PathOnly
		ctx  EvalContext
		pass bool
	}{
		{
			name: "top-level key exists",
			spec: dsl.PathOnly{Path: "kind"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "nested key exists",
			spec: dsl.PathOnly{Path: "spec.replicas"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "array element exists",
			spec: dsl.PathOnly{Path: "spec.containers[0].name"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "null value still counts as existing",
			spec: dsl.PathOnly{Path: "nullField"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "missing key",
			spec: dsl.PathOnly{Path: "metadata"},
			ctx:  EvalContext{Doc: doc},
			pass: false,
		},
		{
			name: "out-of-range array index",
			spec: dsl.PathOnly{Path: "spec.containers[9]"},
			ctx:  EvalContext{Doc: doc},
			pass: false,
		},
		{
			name: "nil doc",
			spec: dsl.PathOnly{Path: "kind"},
			ctx:  EvalContext{Doc: nil},
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalExists(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalNotExists(t *testing.T) {
	doc := map[string]any{
		"kind": "Deployment",
		"spec": map[string]any{"replicas": 3},
	}

	tests := []struct {
		name string
		spec dsl.PathOnly
		ctx  EvalContext
		pass bool
	}{
		{
			name: "key absent — pass",
			spec: dsl.PathOnly{Path: "metadata"},
			ctx:  EvalContext{Doc: doc},
			pass: true,
		},
		{
			name: "key present — fail",
			spec: dsl.PathOnly{Path: "kind"},
			ctx:  EvalContext{Doc: doc},
			pass: false,
		},
		{
			name: "nil doc",
			spec: dsl.PathOnly{Path: "kind"},
			ctx:  EvalContext{Doc: nil},
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalNotExists(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}
