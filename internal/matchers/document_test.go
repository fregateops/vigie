package matchers

import (
	"errors"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestEvalIsKind(t *testing.T) {
	doc := map[string]any{"kind": "Deployment", "apiVersion": "apps/v1"}

	kind := func(s string) *string { return &s }

	tests := []struct {
		name string
		want *string
		ctx  EvalContext
		pass bool
	}{
		{"correct kind", kind("Deployment"), EvalContext{Doc: doc}, true},
		{"wrong kind", kind("Service"), EvalContext{Doc: doc}, false},
		{"nil doc", kind("Deployment"), EvalContext{Doc: nil}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsKind(tt.want, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalIsAPIVersion(t *testing.T) {
	doc := map[string]any{"kind": "Deployment", "apiVersion": "apps/v1"}

	av := func(s string) *string { return &s }

	tests := []struct {
		name string
		want *string
		ctx  EvalContext
		pass bool
	}{
		{"correct apiVersion", av("apps/v1"), EvalContext{Doc: doc}, true},
		{"wrong apiVersion", av("v1"), EvalContext{Doc: doc}, false},
		{"nil doc", av("apps/v1"), EvalContext{Doc: nil}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalIsAPIVersion(tt.want, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalHasDocuments(t *testing.T) {
	count := func(n int) *int { return &n }

	docs0 := []map[string]any{}
	docs1 := []map[string]any{{"kind": "ConfigMap"}}
	docs3 := []map[string]any{{"kind": "A"}, {"kind": "B"}, {"kind": "C"}}

	tests := []struct {
		name string
		want *int
		ctx  EvalContext
		pass bool
	}{
		{"zero docs — match", count(0), EvalContext{Docs: docs0}, true},
		{"one doc — match", count(1), EvalContext{Docs: docs1}, true},
		{"three docs — match", count(3), EvalContext{Docs: docs3}, true},
		{"wrong count", count(2), EvalContext{Docs: docs1}, false},
		{"nil docs treated as empty", count(0), EvalContext{Docs: nil}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalHasDocuments(tt.want, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalFailedTemplate(t *testing.T) {
	renderErr := errors.New("error calling fail: password is required")

	tests := []struct {
		name string
		spec dsl.FailedTemplateSpec
		ctx  EvalContext
		pass bool
	}{
		{
			name: "render failed — no pattern — pass",
			spec: dsl.FailedTemplateSpec{},
			ctx:  EvalContext{RenderErr: renderErr},
			pass: true,
		},
		{
			name: "render succeeded — fail",
			spec: dsl.FailedTemplateSpec{},
			ctx:  EvalContext{RenderErr: nil},
			pass: false,
		},
		{
			name: "error matches pattern — pass",
			spec: dsl.FailedTemplateSpec{ErrorPattern: "password is required"},
			ctx:  EvalContext{RenderErr: renderErr},
			pass: true,
		},
		{
			name: "error does not match pattern — fail",
			spec: dsl.FailedTemplateSpec{ErrorPattern: "some other error"},
			ctx:  EvalContext{RenderErr: renderErr},
			pass: false,
		},
		{
			name: "pattern is regex — anchored match",
			spec: dsl.FailedTemplateSpec{ErrorPattern: "^error calling fail: password is required$"},
			ctx:  EvalContext{RenderErr: renderErr},
			pass: true,
		},
		{
			name: "invalid regex pattern",
			spec: dsl.FailedTemplateSpec{ErrorPattern: "[invalid"},
			ctx:  EvalContext{RenderErr: renderErr},
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalFailedTemplate(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}
