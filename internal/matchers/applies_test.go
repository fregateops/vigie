package matchers

import (
	"errors"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func applyCtx(inApplyTier bool, applyErr error) EvalContext {
	return EvalContext{
		InApplyTier: inApplyTier,
		ApplyError:  applyErr,
	}
}

func statusError(reason metav1.StatusReason, msg string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Reason:  reason,
		Message: msg,
	}}
}

func TestEvalApplies(t *testing.T) {
	someErr := errors.New("webhook denied the request")

	tests := []struct {
		name string
		ctx  EvalContext
		pass bool
	}{
		{
			name: "accepted in apply tier — pass",
			ctx:  applyCtx(true, nil),
			pass: true,
		},
		{
			name: "rejected in apply tier — fail",
			ctx:  applyCtx(true, someErr),
			pass: false,
		},
		{
			name: "outside apply tier — tier mismatch error",
			ctx:  applyCtx(false, nil),
			pass: false,
		},
		{
			name: "outside apply tier with error — still tier mismatch",
			ctx:  applyCtx(false, someErr),
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalApplies(&dsl.AppliesSpec{}, tt.ctx)
			if result.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.pass, result.Message)
			}
			if !tt.pass && tt.ctx.InApplyTier == false && result.Message != errTierMismatch {
				t.Errorf("expected tier mismatch message, got %q", result.Message)
			}
		})
	}
}

func TestEvalRejected(t *testing.T) {
	plainErr := errors.New("no kind PodSecurityPolicy is registered")
	invalidErr := statusError(metav1.StatusReasonInvalid, "spec.replicas: Invalid value: -1")
	forbiddenErr := statusError(metav1.StatusReasonForbidden, "cannot create resource")

	tests := []struct {
		name string
		spec dsl.RejectedSpec
		ctx  EvalContext
		pass bool
	}{
		{
			name: "rejected with any error — pass",
			spec: dsl.RejectedSpec{},
			ctx:  applyCtx(true, plainErr),
			pass: true,
		},
		{
			name: "not rejected — fail",
			spec: dsl.RejectedSpec{},
			ctx:  applyCtx(true, nil),
			pass: false,
		},
		{
			name: "outside apply tier — tier mismatch",
			spec: dsl.RejectedSpec{},
			ctx:  applyCtx(false, plainErr),
			pass: false,
		},
		{
			name: "reason matches StatusError — pass",
			spec: dsl.RejectedSpec{Reason: "Invalid"},
			ctx:  applyCtx(true, invalidErr),
			pass: true,
		},
		{
			name: "reason does not match StatusError — fail",
			spec: dsl.RejectedSpec{Reason: "Forbidden"},
			ctx:  applyCtx(true, invalidErr),
			pass: false,
		},
		{
			name: "reason matches Forbidden — pass",
			spec: dsl.RejectedSpec{Reason: "Forbidden"},
			ctx:  applyCtx(true, forbiddenErr),
			pass: true,
		},
		{
			name: "message regex matches — pass",
			spec: dsl.RejectedSpec{Message: "no kind.*registered"},
			ctx:  applyCtx(true, plainErr),
			pass: true,
		},
		{
			name: "message regex does not match — fail",
			spec: dsl.RejectedSpec{Message: "admission webhook"},
			ctx:  applyCtx(true, plainErr),
			pass: false,
		},
		{
			name: "reason and message both match — pass",
			spec: dsl.RejectedSpec{Reason: "Invalid", Message: "spec.replicas"},
			ctx:  applyCtx(true, invalidErr),
			pass: true,
		},
		{
			name: "reason matches but message does not — fail",
			spec: dsl.RejectedSpec{Reason: "Invalid", Message: "unrelated pattern"},
			ctx:  applyCtx(true, invalidErr),
			pass: false,
		},
		{
			name: "invalid message regex — fail with error",
			spec: dsl.RejectedSpec{Message: "[invalid"},
			ctx:  applyCtx(true, plainErr),
			pass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalRejected(&tt.spec, tt.ctx)
			if result.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.pass, result.Message)
			}
			if !tt.pass && !tt.ctx.InApplyTier && result.Message != errTierMismatch {
				t.Errorf("expected tier mismatch message, got %q", result.Message)
			}
		})
	}
}
