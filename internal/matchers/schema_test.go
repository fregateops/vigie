package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

var schemaDoc = map[string]any{
	"kind": "Deployment",
	"spec": map[string]any{
		"replicas": 3,
		"template": map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":  "app",
						"image": "myrepo/myapp:latest",
					},
				},
			},
		},
	},
}

func TestEvalMatchSchema_Pass(t *testing.T) {
	spec := &dsl.MatchSchemaSpec{
		Path: "spec",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"replicas": map[string]any{"type": "integer"},
			},
			"required": []any{"replicas"},
		},
	}
	r := evalMatchSchema(spec, EvalContext{Doc: schemaDoc})
	if !r.Pass {
		t.Errorf("expected pass, got: %s", r.Message)
	}
}

func TestEvalMatchSchema_Fail_WrongType(t *testing.T) {
	spec := &dsl.MatchSchemaSpec{
		Path: "spec",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"replicas": map[string]any{"type": "string"},
			},
			"required": []any{"replicas"},
		},
	}
	r := evalMatchSchema(spec, EvalContext{Doc: schemaDoc})
	if r.Pass {
		t.Error("expected failure for wrong type")
	}
	if r.Message == "" {
		t.Error("expected non-empty failure message")
	}
}

func TestEvalMatchSchema_Fail_MissingRequired(t *testing.T) {
	spec := &dsl.MatchSchemaSpec{
		Path: "spec",
		Schema: map[string]any{
			"type":     "object",
			"required": []any{"replicas", "strategy"},
		},
	}
	r := evalMatchSchema(spec, EvalContext{Doc: schemaDoc})
	if r.Pass {
		t.Error("expected failure for missing required field 'strategy'")
	}
}

func TestEvalMatchSchema_PathNotFound(t *testing.T) {
	spec := &dsl.MatchSchemaSpec{
		Path:   "spec.nonexistent",
		Schema: map[string]any{"type": "object"},
	}
	r := evalMatchSchema(spec, EvalContext{Doc: schemaDoc})
	if r.Pass {
		t.Error("expected failure for non-existent path")
	}
}

func TestEvalMatchSchema_RootDoc(t *testing.T) {
	spec := &dsl.MatchSchemaSpec{
		Path: ".",
		Schema: map[string]any{
			"type":     "object",
			"required": []any{"kind"},
		},
	}
	r := evalMatchSchema(spec, EvalContext{Doc: schemaDoc})
	if !r.Pass {
		t.Errorf("expected pass for root doc with kind present, got: %s", r.Message)
	}
}
