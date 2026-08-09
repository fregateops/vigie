package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

var quantifierDoc = map[string]any{
	"kind":     "Deployment",
	"replicas": 3,
}

func TestEvalAllOf(t *testing.T) {
	kindMatch := dsl.Assertion{IsKind: strPtr("Deployment")}
	kindMiss := dsl.Assertion{IsKind: strPtr("Service")}
	ctx := EvalContext{Doc: quantifierDoc}

	tests := []struct {
		name    string
		specs   []dsl.Assertion
		pass    bool
		wantMsg string
	}{
		{"all pass", []dsl.Assertion{kindMatch, kindMatch}, true, ""},
		{"first fails", []dsl.Assertion{kindMiss, kindMatch}, false, "allOf"},
		{"second fails", []dsl.Assertion{kindMatch, kindMiss}, false, "allOf"},
		{"all fail", []dsl.Assertion{kindMiss, kindMiss}, false, "allOf"},
		{"empty specs — vacuously pass", []dsl.Assertion{}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalAllOf(tt.specs, ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
			if !r.Pass && r.Message == "" {
				t.Error("expected non-empty failure message")
			}
		})
	}
}

func TestEvalAllOf_ReportsAllFailures(t *testing.T) {
	kindMiss := dsl.Assertion{IsKind: strPtr("Service")}
	ctx := EvalContext{Doc: quantifierDoc}

	r := evalAllOf([]dsl.Assertion{kindMiss, kindMiss}, ctx)
	if r.Pass {
		t.Fatal("expected failure")
	}
	// Message should mention "2 of 2".
	if r.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestEvalAnyOf(t *testing.T) {
	kindMatch := dsl.Assertion{IsKind: strPtr("Deployment")}
	kindMiss := dsl.Assertion{IsKind: strPtr("Service")}
	ctx := EvalContext{Doc: quantifierDoc}

	tests := []struct {
		name  string
		specs []dsl.Assertion
		pass  bool
	}{
		{"all pass — first wins", []dsl.Assertion{kindMatch, kindMatch}, true},
		{"first passes", []dsl.Assertion{kindMatch, kindMiss}, true},
		{"second passes", []dsl.Assertion{kindMiss, kindMatch}, true},
		{"none pass", []dsl.Assertion{kindMiss, kindMiss}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalAnyOf(tt.specs, ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
			if !r.Pass && r.Message == "" {
				t.Error("expected non-empty failure message")
			}
		})
	}
}
