package matchers

import (
	"context"
	"fmt"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/path"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func init() {
	Register(simpleMatcher{
		name:     "lookup",
		matches:  func(a dsl.Assertion) bool { return a.Lookup != nil },
		tiers:    Tiers(TierSimulated, TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalLookup(a.Lookup, ctx) },
	})
}

func evalLookup(spec *dsl.LookupAssert, ctx EvalContext) Result {
	if !ctx.InApplyTier {
		return Result{Pass: false, Message: errApplyTierRequired}
	}
	if ctx.RESTConfig == nil {
		return Result{Pass: false, Message: "lookup: no REST config available"}
	}

	within, err := parseTimeout(spec.Within, defaultWaitTimeout)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("lookup: invalid within %q: %v", spec.Within, err)}
	}

	gvr, err := kindToGVR(spec.Kind)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("lookup: %v", err)}
	}

	dynClient, err := dynamic.NewForConfig(ctx.RESTConfig)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("lookup: failed to create dynamic client: %v", err)}
	}

	if spec.LabelSelector != "" {
		return evalLookupList(spec, ctx, dynClient, gvr, within)
	}
	return evalLookupSingle(spec, ctx, dynClient, gvr, within)
}

func evalLookupSingle(spec *dsl.LookupAssert, ctx EvalContext, dynClient dynamic.Interface, gvr schema.GroupVersionResource, within time.Duration) Result {
	deadline := time.Now().Add(within)
	pollCtx, cancelPoll := context.WithDeadline(context.Background(), deadline)
	defer cancelPoll()

	ns := resolveNS(spec.Namespace, ctx)
	var lastObj map[string]any

	for {
		obj, fetchErr := fetchResource(pollCtx, dynClient, gvr, ns, spec.Name)
		if fetchErr == nil {
			lastObj = obj
			if spec.Until == nil || untilConditionMet(obj, spec.Until) {
				break
			}
		}

		if time.Now().After(deadline) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("lookup: timeout waiting for %s/%s until condition; last object: %v", spec.Kind, spec.Name, lastObj),
			}
		}
		time.Sleep(pollInterval)
	}

	return runThenAssertions(spec.Then, lastObj, ctx)
}

func evalLookupList(spec *dsl.LookupAssert, ctx EvalContext, dynClient dynamic.Interface, gvr schema.GroupVersionResource, within time.Duration) Result {
	deadline := time.Now().Add(within)
	pollCtx, cancelPoll := context.WithDeadline(context.Background(), deadline)
	defer cancelPoll()

	ns := resolveNS(spec.Namespace, ctx)
	var items []map[string]any

	for {
		res := dynamicResourceFor(dynClient, gvr, ns)
		list, listErr := res.List(pollCtx, metav1.ListOptions{
			LabelSelector: spec.LabelSelector,
		})
		if listErr == nil && len(list.Items) > 0 {
			items = make([]map[string]any, 0, len(list.Items))
			for idx := range list.Items {
				items = append(items, list.Items[idx].Object)
			}
			break
		}

		if time.Now().After(deadline) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("lookup: timeout: no %s resources found matching labelSelector %q", spec.Kind, spec.LabelSelector),
			}
		}
		time.Sleep(pollInterval)
	}

	for itemIdx, item := range items {
		for assertIdx, assertion := range spec.ForEach {
			itemCtx := ctx
			itemCtx.Doc = item
			result := Evaluate(assertion, itemCtx)
			if !result.Pass {
				return Result{
					Pass:    false,
					Message: fmt.Sprintf("lookup forEach[item=%d, assert=%d]: %s", itemIdx, assertIdx, result.Message),
				}
			}
		}
	}
	return Result{Pass: true}
}

// untilConditionMet checks whether the path in obj equals the expected value.
// TODO: DESIGN.md §12 specifies comparator syntax (gte, lte, etc.) in until:;
// only equality is implemented here. Extend PathValue or add a UntilSpec type
// with comparator fields when richer comparisons are needed.
func untilConditionMet(obj map[string]any, until *dsl.PathValue) bool {
	val, found, err := path.Get(obj, until.Path)
	if err != nil || !found {
		return false
	}
	return deepEqual(val, until.Value)
}

func runThenAssertions(assertions []dsl.Assertion, obj map[string]any, ctx EvalContext) Result {
	for assertIdx, assertion := range assertions {
		assertCtx := ctx
		assertCtx.Doc = obj
		result := Evaluate(assertion, assertCtx)
		if !result.Pass {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("lookup then[%d]: %s", assertIdx, result.Message),
			}
		}
	}
	return Result{Pass: true}
}
