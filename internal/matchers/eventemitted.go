package matchers

import (
	"context"
	"fmt"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func init() {
	Register(simpleMatcher{
		name:     "eventEmitted",
		matches:  func(a dsl.Assertion) bool { return a.EventEmitted != nil },
		tiers:    Tiers(TierSimulated, TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalEventEmitted(a.EventEmitted, ctx) },
	})
}

func evalEventEmitted(spec *dsl.EventAssert, ctx EvalContext) Result {
	if !ctx.InApplyTier {
		return Result{Pass: false, Message: errApplyTierRequired}
	}
	if ctx.RESTConfig == nil {
		return Result{Pass: false, Message: "eventEmitted: no REST config available"}
	}

	within, err := parseTimeout(spec.Within, defaultWaitTimeout)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("eventEmitted: invalid within %q: %v", spec.Within, err)}
	}

	clientset, err := kubernetes.NewForConfig(ctx.RESTConfig)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("eventEmitted: failed to create kubernetes client: %v", err)}
	}

	namespace := resolveNS(spec.InvolvedObject.Namespace, ctx)
	deadline := time.Now().Add(within)
	pollCtx, cancelPoll := context.WithDeadline(context.Background(), deadline)
	defer cancelPoll()

	for {
		found, checkErr := findMatchingEvent(pollCtx, clientset, namespace, spec)
		if checkErr != nil {
			return Result{Pass: false, Message: fmt.Sprintf("eventEmitted: error listing events: %v", checkErr)}
		}
		if found {
			return Result{Pass: true}
		}

		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	return Result{
		Pass: false,
		Message: fmt.Sprintf("eventEmitted: timeout waiting for event (involvedObject: %s/%s, reason: %q, type: %q)",
			spec.InvolvedObject.Kind, spec.InvolvedObject.Name, spec.Reason, spec.Type),
	}
}

func findMatchingEvent(ctx context.Context, clientset kubernetes.Interface, namespace string, spec *dsl.EventAssert) (bool, error) {
	events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, event := range events.Items {
		if !matchesEventFilter(event.InvolvedObject.Kind, spec.InvolvedObject.Kind) {
			continue
		}
		if !matchesEventFilter(event.InvolvedObject.Name, spec.InvolvedObject.Name) {
			continue
		}
		if !matchesEventFilter(event.Reason, spec.Reason) {
			continue
		}
		if !matchesEventFilter(event.Type, spec.Type) {
			continue
		}
		return true, nil
	}
	return false, nil
}

// matchesEventFilter returns true when filter is empty (matches any) or equals the event field.
func matchesEventFilter(eventField, filter string) bool {
	return filter == "" || eventField == filter
}
