package matchers

import (
	"testing"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestEvalWaitFor_NotInApplyTier(t *testing.T) {
	spec := &dsl.WaitForSpec{Kind: "Deployment", Name: "my-app", Condition: "Available"}
	result := evalWaitFor(spec, EvalContext{InApplyTier: false})
	if result.Pass {
		t.Error("expected fail when not in apply tier")
	}
	if result.Message != errApplyTierRequired {
		t.Errorf("expected tier mismatch message, got %q", result.Message)
	}
}

func TestEvalWaitFor_NoRESTConfig(t *testing.T) {
	spec := &dsl.WaitForSpec{Kind: "Deployment", Name: "my-app", Condition: "Available"}
	result := evalWaitFor(spec, EvalContext{InApplyTier: true, RESTConfig: nil})
	if result.Pass {
		t.Error("expected fail when REST config is nil")
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestEvalWaitFor_InvalidTimeout(t *testing.T) {
	spec := &dsl.WaitForSpec{Kind: "Deployment", Name: "my-app", Condition: "Available", Timeout: "notaduration"}
	result := evalWaitFor(spec, EvalContext{InApplyTier: true, RESTConfig: nil})
	// RESTConfig check happens before timeout parse; still fails with no REST config
	if result.Pass {
		t.Error("expected fail")
	}
}

func TestEvalBecomesReady_NotInApplyTier(t *testing.T) {
	spec := &dsl.WaitForSpec{Kind: "Deployment", Name: "my-app"}
	result := evalBecomesReady(spec, EvalContext{InApplyTier: false})
	if result.Pass {
		t.Error("expected fail when not in apply tier")
	}
	if result.Message != errApplyTierRequired {
		t.Errorf("expected tier mismatch message, got %q", result.Message)
	}
}

func TestEvalBecomesReady_NoRESTConfig(t *testing.T) {
	spec := &dsl.WaitForSpec{Kind: "Deployment", Name: "my-app"}
	result := evalBecomesReady(spec, EvalContext{InApplyTier: true, RESTConfig: nil})
	if result.Pass {
		t.Error("expected fail when REST config is nil")
	}
}

func TestIsResourceReady_Deployment(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]any
		want   bool
	}{
		{
			name:   "fully available",
			status: map[string]any{"replicas": float64(3), "availableReplicas": float64(3)},
			want:   true,
		},
		{
			name:   "partially available",
			status: map[string]any{"replicas": float64(3), "availableReplicas": float64(1)},
			want:   false,
		},
		{
			name:   "zero replicas",
			status: map[string]any{"replicas": float64(0), "availableReplicas": float64(0)},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]any{"status": tt.status}
			ready, err := isResourceReady(obj, "Deployment")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.want {
				t.Errorf("ready=%v want=%v", ready, tt.want)
			}
		})
	}
}

func TestIsResourceReady_StatefulSet(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]any
		want   bool
	}{
		{
			name:   "all ready",
			status: map[string]any{"replicas": float64(2), "readyReplicas": float64(2)},
			want:   true,
		},
		{
			name:   "none ready",
			status: map[string]any{"replicas": float64(2), "readyReplicas": float64(0)},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]any{"status": tt.status}
			ready, err := isResourceReady(obj, "StatefulSet")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.want {
				t.Errorf("ready=%v want=%v", ready, tt.want)
			}
		})
	}
}

func TestIsResourceReady_Pod(t *testing.T) {
	readyCondition := map[string]any{"type": "Ready", "status": "True"}
	notReadyCondition := map[string]any{"type": "Ready", "status": "False"}

	tests := []struct {
		name   string
		status map[string]any
		want   bool
	}{
		{
			name: "running and ready",
			status: map[string]any{
				"phase":      "Running",
				"conditions": []any{readyCondition},
			},
			want: true,
		},
		{
			name: "running but not ready",
			status: map[string]any{
				"phase":      "Running",
				"conditions": []any{notReadyCondition},
			},
			want: false,
		},
		{
			name: "not running",
			status: map[string]any{
				"phase":      "Pending",
				"conditions": []any{readyCondition},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]any{"status": tt.status}
			ready, err := isResourceReady(obj, "Pod")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.want {
				t.Errorf("ready=%v want=%v", ready, tt.want)
			}
		})
	}
}

func TestIsResourceReady_Job(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]any
		want   bool
	}{
		{
			name:   "succeeded",
			status: map[string]any{"succeeded": float64(1)},
			want:   true,
		},
		{
			name:   "not succeeded",
			status: map[string]any{"succeeded": float64(0)},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]any{"status": tt.status}
			ready, err := isResourceReady(obj, "Job")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.want {
				t.Errorf("ready=%v want=%v", ready, tt.want)
			}
		})
	}
}

func TestIsResourceReady_DaemonSet(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]any
		want   bool
	}{
		{
			name:   "all ready",
			status: map[string]any{"desiredNumberScheduled": float64(3), "numberReady": float64(3)},
			want:   true,
		},
		{
			name:   "partial",
			status: map[string]any{"desiredNumberScheduled": float64(3), "numberReady": float64(2)},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]any{"status": tt.status}
			ready, err := isResourceReady(obj, "DaemonSet")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.want {
				t.Errorf("ready=%v want=%v", ready, tt.want)
			}
		})
	}
}

func TestIsResourceReady_UnsupportedKind(t *testing.T) {
	obj := map[string]any{"status": map[string]any{}}
	_, err := isResourceReady(obj, "CustomResource")
	if err == nil {
		t.Error("expected error for unsupported kind")
	}
}

func TestIsResourceReady_NoStatus(t *testing.T) {
	obj := map[string]any{}
	ready, err := isResourceReady(obj, "Deployment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not ready when no status field")
	}
}

func TestConditionMet(t *testing.T) {
	conditions := []any{
		map[string]any{"type": "Available", "status": "True"},
		map[string]any{"type": "Progressing", "status": "True"},
		map[string]any{"type": "ReplicaFailure", "status": "False"},
	}

	if !conditionMet(conditions, "Available") {
		t.Error("expected Available condition to be met")
	}
	if conditionMet(conditions, "ReplicaFailure") {
		t.Error("expected ReplicaFailure condition not to be met (status is False)")
	}
	if conditionMet(conditions, "Ready") {
		t.Error("expected Ready condition not to be met (not present)")
	}
}

func TestParseTimeout(t *testing.T) {
	fiveMinutes := 5 * time.Minute

	tests := []struct {
		input   string
		wantErr bool
		want    time.Duration
	}{
		{"", false, fiveMinutes},
		{"30s", false, 30 * time.Second},
		{"2m", false, 2 * time.Minute},
		{"notaduration", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			dur, err := parseTimeout(tt.input, fiveMinutes)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && dur != tt.want {
				t.Errorf("duration=%v want=%v", dur, tt.want)
			}
		})
	}
}
