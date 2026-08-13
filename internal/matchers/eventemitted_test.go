package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestEvalEventEmitted_NotInApplyTier(t *testing.T) {
	spec := &dsl.EventAssert{
		InvolvedObject: dsl.EventObjectRef{Kind: "Deployment", Name: "api"},
		Reason:         "ScalingReplicaSet",
		Type:           "Normal",
	}
	result := evalEventEmitted(spec, EvalContext{InApplyTier: false})
	if result.Pass {
		t.Error("expected fail when not in apply tier")
	}
	if result.Message != errApplyTierRequired {
		t.Errorf("expected tier mismatch message, got %q", result.Message)
	}
}

func TestEvalEventEmitted_NoRESTConfig(t *testing.T) {
	spec := &dsl.EventAssert{
		InvolvedObject: dsl.EventObjectRef{Kind: "Deployment", Name: "api"},
		Reason:         "ScalingReplicaSet",
	}
	result := evalEventEmitted(spec, EvalContext{InApplyTier: true, RESTConfig: nil})
	if result.Pass {
		t.Error("expected fail when REST config is nil")
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestMatchesEventFilter(t *testing.T) {
	tests := []struct {
		name        string
		eventField  string
		filter      string
		wantMatches bool
	}{
		{
			name:        "empty filter matches anything",
			eventField:  "SomeValue",
			filter:      "",
			wantMatches: true,
		},
		{
			name:        "exact match",
			eventField:  "ScalingReplicaSet",
			filter:      "ScalingReplicaSet",
			wantMatches: true,
		},
		{
			name:        "no match",
			eventField:  "BackOff",
			filter:      "ScalingReplicaSet",
			wantMatches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesEventFilter(tt.eventField, tt.filter)
			if got != tt.wantMatches {
				t.Errorf("matchesEventFilter(%q, %q)=%v want=%v", tt.eventField, tt.filter, got, tt.wantMatches)
			}
		})
	}
}
