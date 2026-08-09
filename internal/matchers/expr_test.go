package matchers

import (
	"testing"
)

func TestEvalExpr(t *testing.T) {
	doc := map[string]any{
		"kind":     "Deployment",
		"replicas": 3,
	}
	docs := []map[string]any{doc}
	expr := func(s string) *string { return &s }

	tests := []struct {
		name string
		spec *string
		ctx  EvalContext
		pass bool
	}{
		{
			"true expression — pass",
			expr(`doc.kind == "Deployment"`),
			EvalContext{Doc: doc, Docs: docs},
			true,
		},
		{
			"false expression — fail",
			expr(`doc.kind == "Service"`),
			EvalContext{Doc: doc, Docs: docs},
			false,
		},
		{
			"arithmetic comparison",
			expr(`doc.replicas > 1`),
			EvalContext{Doc: doc, Docs: docs},
			true,
		},
		{
			"resources binding — Deployment in resources",
			expr(`"Deployment" in resources`),
			EvalContext{Doc: doc, Docs: docs},
			true,
		},
		{
			"matrix binding",
			expr(`matrix.tier == "large"`),
			EvalContext{Doc: doc, Docs: docs, MatrixEntry: map[string]any{"tier": "large"}},
			true,
		},
		{
			"case binding",
			expr(`case.name == "golden"`),
			EvalContext{Doc: doc, Docs: docs, CaseEntry: map[string]any{"name": "golden"}},
			true,
		},
		{
			"output binding for helper tests",
			expr(`output == "hello"`),
			EvalContext{IsHelperTest: true, HelperOutput: "hello"},
			true,
		},
		{
			"non-bool result — fail",
			expr(`doc.kind`),
			EvalContext{Doc: doc, Docs: docs},
			false,
		},
		{
			"invalid CEL expression — fail",
			expr(`!!invalid CEL!!`),
			EvalContext{Doc: doc, Docs: docs},
			false,
		},
		{
			"empty resources when no docs",
			expr(`!("Deployment" in resources)`),
			EvalContext{Doc: nil, Docs: nil},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalExpr(tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
			if !r.Pass && r.Message == "" {
				t.Error("expected non-empty failure message")
			}
		})
	}
}
