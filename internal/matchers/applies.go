package matchers

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/fregateops/vigie/internal/dsl"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func init() {
	Register(simpleMatcher{
		name:     "applies",
		matches:  func(a dsl.Assertion) bool { return a.Applies != nil },
		tiers:    Tiers(TierAPIServer, TierSimulated, TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalApplies(a.Applies, ctx) },
	})
	Register(simpleMatcher{
		name:     "rejected",
		matches:  func(a dsl.Assertion) bool { return a.Rejected != nil },
		tiers:    Tiers(TierAPIServer, TierSimulated, TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalRejected(a.Rejected, ctx) },
	})
}

const errTierMismatch = "applies/rejected matchers require a cluster backend (run test with --cluster)"

func evalApplies(_ *dsl.AppliesSpec, ctx EvalContext) Result {
	if !ctx.InApplyTier {
		return Result{Pass: false, Message: errTierMismatch}
	}
	if ctx.ApplyError != nil {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("applies: resource was rejected: %v", ctx.ApplyError),
		}
	}
	return Result{Pass: true}
}

func evalRejected(spec *dsl.RejectedSpec, ctx EvalContext) Result {
	if !ctx.InApplyTier {
		return Result{Pass: false, Message: errTierMismatch}
	}
	if ctx.ApplyError == nil {
		return Result{Pass: false, Message: "rejected: resource was accepted but expected rejection"}
	}

	if spec.Reason != "" {
		if !matchesReason(ctx.ApplyError, spec.Reason) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("rejected: error reason %q does not match expected %q (error: %v)", extractReason(ctx.ApplyError), spec.Reason, ctx.ApplyError),
			}
		}
	}

	if spec.Message != "" {
		re, err := regexp.Compile(spec.Message)
		if err != nil {
			return Result{Pass: false, Message: fmt.Sprintf("rejected: invalid message regex %q: %v", spec.Message, err)}
		}
		if !re.MatchString(ctx.ApplyError.Error()) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("rejected: error message %q does not match pattern %q", ctx.ApplyError.Error(), spec.Message),
			}
		}
	}

	return Result{Pass: true}
}

// matchesReason checks whether the error carries a Kubernetes API status reason
// matching the expected value. Falls back to a substring match on the error
// message for non-status errors so tests work with plain errors too.
func matchesReason(err error, expected string) bool {
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return string(statusErr.ErrStatus.Reason) == expected
	}
	return fmt.Sprintf("%v", err) == expected
}

// extractReason returns the Kubernetes API status reason string from err, or
// an empty string when the error is not a StatusError.
func extractReason(err error) string {
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return string(statusErr.ErrStatus.Reason)
	}
	return ""
}
