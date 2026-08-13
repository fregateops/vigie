package matchers

import (
	"context"
	"fmt"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func init() {
	Register(simpleMatcher{
		name:     "waitFor",
		matches:  func(a dsl.Assertion) bool { return a.WaitFor != nil },
		tiers:    Tiers(TierSimulated, TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalWaitFor(a.WaitFor, ctx) },
	})
	Register(simpleMatcher{
		name:     "becomesReady",
		matches:  func(a dsl.Assertion) bool { return a.BecomesReady != nil },
		tiers:    Tiers(TierSimulated, TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalBecomesReady(a.BecomesReady, ctx) },
	})
}

// waitForSetup holds the validated inputs shared by evalWaitFor and evalBecomesReady.
type waitForSetup struct {
	dynClient dynamic.Interface
	gvr       schema.GroupVersionResource
	timeout   time.Duration
}

// setupWaitForMatcher validates the common preamble for waitFor and becomesReady:
// tier guard, REST config, timeout parse, and kind lookup. Returns a non-nil *Result
// on any validation failure so the caller can return early.
func setupWaitForMatcher(matcherName string, spec *dsl.WaitForSpec, ctx EvalContext) (*waitForSetup, *Result) {
	if !ctx.InApplyTier {
		r := Result{Pass: false, Message: errApplyTierRequired}
		return nil, &r
	}
	if ctx.RESTConfig == nil {
		r := Result{Pass: false, Message: matcherName + ": no REST config available"}
		return nil, &r
	}
	timeout, err := parseTimeout(spec.Timeout, defaultWaitTimeout)
	if err != nil {
		r := Result{Pass: false, Message: fmt.Sprintf("%s: invalid timeout %q: %v", matcherName, spec.Timeout, err)}
		return nil, &r
	}
	gvr, err := kindToGVR(spec.Kind)
	if err != nil {
		r := Result{Pass: false, Message: fmt.Sprintf("%s: %v", matcherName, err)}
		return nil, &r
	}
	dynClient, err := dynamic.NewForConfig(ctx.RESTConfig)
	if err != nil {
		r := Result{Pass: false, Message: fmt.Sprintf("%s: failed to create dynamic client: %v", matcherName, err)}
		return nil, &r
	}
	return &waitForSetup{dynClient: dynClient, gvr: gvr, timeout: timeout}, nil
}

func evalWaitFor(spec *dsl.WaitForSpec, ctx EvalContext) Result {
	setup, earlyResult := setupWaitForMatcher("waitFor", spec, ctx)
	if earlyResult != nil {
		return *earlyResult
	}

	deadline := time.Now().Add(setup.timeout)
	pollCtx, cancelPoll := context.WithDeadline(context.Background(), deadline)
	defer cancelPoll()

	ns := resolveNS(spec.Namespace, ctx)
	var lastConditions []any

	for {
		obj, fetchErr := fetchResource(pollCtx, setup.dynClient, setup.gvr, ns, spec.Name)
		if fetchErr == nil {
			conditions, found := extractConditions(obj)
			if found {
				lastConditions = conditions
				if conditionMet(conditions, spec.Condition) {
					return Result{Pass: true}
				}
			}
		}

		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	return Result{
		Pass:    false,
		Message: fmt.Sprintf("waitFor: timeout waiting for %s/%s condition %q; last conditions: %v", spec.Kind, spec.Name, spec.Condition, lastConditions),
	}
}

func evalBecomesReady(spec *dsl.WaitForSpec, ctx EvalContext) Result {
	setup, earlyResult := setupWaitForMatcher("becomesReady", spec, ctx)
	if earlyResult != nil {
		return *earlyResult
	}

	deadline := time.Now().Add(setup.timeout)
	pollCtx, cancelPoll := context.WithDeadline(context.Background(), deadline)
	defer cancelPoll()

	ns := resolveNS(spec.Namespace, ctx)

	for {
		obj, fetchErr := fetchResource(pollCtx, setup.dynClient, setup.gvr, ns, spec.Name)
		if fetchErr == nil {
			ready, checkErr := isResourceReady(obj, spec.Kind)
			if checkErr != nil {
				return Result{Pass: false, Message: fmt.Sprintf("becomesReady: %v", checkErr)}
			}
			if ready {
				return Result{Pass: true}
			}
		}

		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	return Result{
		Pass:    false,
		Message: fmt.Sprintf("becomesReady: timeout waiting for %s/%s to become ready", spec.Kind, spec.Name),
	}
}

// isResourceReady checks readiness based on resource kind.
func isResourceReady(obj map[string]any, kind string) (bool, error) {
	statusRaw, exists := obj["status"]
	if !exists {
		return false, nil
	}
	status, ok := statusRaw.(map[string]any)
	if !ok {
		return false, nil
	}

	switch kind {
	case "Deployment":
		return deploymentReady(status), nil
	case "StatefulSet":
		return statefulSetReady(status), nil
	case "Pod":
		return podReady(status), nil
	case "Job":
		return jobReady(status), nil
	case "DaemonSet":
		return daemonSetReady(status), nil
	default:
		return false, fmt.Errorf("unsupported kind %q (supported: Deployment, StatefulSet, Pod, Job, DaemonSet)", kind)
	}
}

func deploymentReady(status map[string]any) bool {
	available := toInt64(status["availableReplicas"])
	replicas := toInt64(status["replicas"])
	return replicas > 0 && available == replicas
}

func statefulSetReady(status map[string]any) bool {
	readyReplicas := toInt64(status["readyReplicas"])
	replicas := toInt64(status["replicas"])
	return replicas > 0 && readyReplicas == replicas
}

func podReady(status map[string]any) bool {
	phase, _ := status["phase"].(string)
	if phase != "Running" {
		return false
	}
	conditions, ok := status["conditions"].([]any)
	if !ok {
		return false
	}
	for _, condRaw := range conditions {
		cond, ok := condRaw.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == "Ready" {
			return cond["status"] == "True"
		}
	}
	return false
}

func jobReady(status map[string]any) bool {
	return toInt64(status["succeeded"]) >= 1
}

func daemonSetReady(status map[string]any) bool {
	ready := toInt64(status["numberReady"])
	desired := toInt64(status["desiredNumberScheduled"])
	return desired > 0 && ready == desired
}

func conditionMet(conditions []any, conditionType string) bool {
	for _, condRaw := range conditions {
		cond, ok := condRaw.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == conditionType && cond["status"] == "True" {
			return true
		}
	}
	return false
}

func extractConditions(obj map[string]any) ([]any, bool) {
	statusRaw, exists := obj["status"]
	if !exists {
		return nil, false
	}
	status, ok := statusRaw.(map[string]any)
	if !ok {
		return nil, false
	}
	conditions, ok := status["conditions"].([]any)
	return conditions, ok
}

func toInt64(val any) int64 {
	if val == nil {
		return 0
	}
	switch num := val.(type) {
	case int64:
		return num
	case int32:
		return int64(num)
	case int:
		return int64(num)
	case float64:
		return int64(num)
	case float32:
		return int64(num)
	}
	return 0
}
