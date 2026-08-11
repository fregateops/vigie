package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	"k8s.io/client-go/rest"
)

func TestEvalLookup_NotInApplyTier(t *testing.T) {
	spec := &dsl.LookupAssert{Kind: "ConfigMap", Name: "my-config"}
	result := evalLookup(spec, EvalContext{InApplyTier: false})
	if result.Pass {
		t.Error("expected fail when not in apply tier")
	}
	if result.Message != errApplyTierRequired {
		t.Errorf("expected tier mismatch message, got %q", result.Message)
	}
}

func TestEvalLookup_NoRESTConfig(t *testing.T) {
	spec := &dsl.LookupAssert{Kind: "ConfigMap", Name: "my-config"}
	result := evalLookup(spec, EvalContext{InApplyTier: true, RESTConfig: nil})
	if result.Pass {
		t.Error("expected fail when REST config is nil")
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestEvalLookup_UnknownKind(t *testing.T) {
	spec := &dsl.LookupAssert{Kind: "UnknownCustomResource", Name: "thing"}
	// Use a non-nil RESTConfig so the check reaches the unknown-kind guard.
	result := evalLookup(spec, EvalContext{InApplyTier: true, RESTConfig: &rest.Config{Host: "https://example.invalid"}})
	if result.Pass {
		t.Error("expected fail for unknown kind")
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestUntilConditionMet(t *testing.T) {
	obj := map[string]any{
		"status": map[string]any{
			"currentReplicas": float64(3),
		},
	}

	tests := []struct {
		name  string
		until *dsl.PathValue
		want  bool
	}{
		{
			name:  "matches value",
			until: &dsl.PathValue{Path: "status.currentReplicas", Value: float64(3)},
			want:  true,
		},
		{
			name:  "does not match",
			until: &dsl.PathValue{Path: "status.currentReplicas", Value: float64(5)},
			want:  false,
		},
		{
			name:  "path not found",
			until: &dsl.PathValue{Path: "status.missingField", Value: float64(1)},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := untilConditionMet(obj, tt.until)
			if got != tt.want {
				t.Errorf("untilConditionMet=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestRunThenAssertions_Pass(t *testing.T) {
	obj := map[string]any{"status": map[string]any{"phase": "Running"}}
	ctx := EvalContext{InApplyTier: true}
	assertions := []dsl.Assertion{
		{Equal: &dsl.PathValue{Path: "status.phase", Value: "Running"}},
	}
	result := runThenAssertions(assertions, obj, ctx)
	if !result.Pass {
		t.Errorf("expected pass, got message: %s", result.Message)
	}
}

func TestRunThenAssertions_Fail(t *testing.T) {
	obj := map[string]any{"status": map[string]any{"phase": "Pending"}}
	ctx := EvalContext{InApplyTier: true}
	assertions := []dsl.Assertion{
		{Equal: &dsl.PathValue{Path: "status.phase", Value: "Running"}},
	}
	result := runThenAssertions(assertions, obj, ctx)
	if result.Pass {
		t.Error("expected fail when assertion does not match")
	}
	if result.Message == "" {
		t.Error("expected non-empty message on failure")
	}
}

func TestRunThenAssertions_Empty(t *testing.T) {
	obj := map[string]any{"kind": "ConfigMap"}
	result := runThenAssertions(nil, obj, EvalContext{InApplyTier: true})
	if !result.Pass {
		t.Errorf("expected pass with empty assertions, got message: %s", result.Message)
	}
}
