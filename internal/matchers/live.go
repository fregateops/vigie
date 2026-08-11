package matchers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
)

// Shared helpers for apply-tier matchers that talk to a live API server via the
// dynamic client. The lookup matcher (api tier) uses them now; the M7
// live-cluster matchers (waitFor, becomesReady, eventEmitted) build on the same
// primitives.

const errApplyTierRequired = "apply-tier matchers are not valid in the unit test tier"

const defaultWaitTimeout = 2 * time.Minute
const pollInterval = 2 * time.Second

// clusterScopedResources lists resource names (plural) that are cluster-scoped
// and must be fetched without a namespace prefix.
var clusterScopedResources = map[string]bool{
	"namespaces":          true,
	"clusterroles":        true,
	"clusterrolebindings": true,
}

// dynamicResourceFor returns a ResourceInterface scoped to the namespace for
// namespace-scoped resources, or unscoped for cluster-scoped resources.
func dynamicResourceFor(dynClient dynamic.Interface, gvr schema.GroupVersionResource, namespace string) dynamic.ResourceInterface {
	if clusterScopedResources[gvr.Resource] {
		return dynClient.Resource(gvr)
	}
	return dynClient.Resource(gvr).Namespace(namespace)
}

func fetchResource(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string) (map[string]any, error) {
	unstr, err := dynamicResourceFor(dynClient, gvr, namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return unstr.Object, nil
}

func parseTimeout(s string, fallback time.Duration) (time.Duration, error) {
	if s == "" {
		return fallback, nil
	}
	return time.ParseDuration(s)
}

// builtinKindToGVR maps every built-in Kubernetes Kind to its preferred
// GroupVersionResource, derived once from the compiled-in client-go scheme.
// This covers all built-in kinds statically (no discovery round-trip); CRDs
// are not registered in the scheme and stay unresolvable here.
var builtinKindToGVR = buildBuiltinKindToGVR()

// buildBuiltinKindToGVR walks the client-go scheme's known types and, for each
// Kind, keeps the most stable/preferred GroupVersionKind (see preferGVK), then
// guesses its resource name. Collapsing multi-version kinds this way yields the
// expected GA resources: Deployment->apps/v1, HorizontalPodAutoscaler->
// autoscaling/v2, Ingress->networking.k8s.io/v1.
func buildBuiltinKindToGVR() map[string]schema.GroupVersionResource {
	preferred := map[string]schema.GroupVersionKind{}
	for gvk := range scheme.Scheme.AllKnownTypes() {
		if gvk.Version == runtime.APIVersionInternal {
			continue
		}
		// List wrappers (PodList, ...) are not addressable resources.
		if strings.HasSuffix(gvk.Kind, "List") {
			continue
		}
		if best, seen := preferred[gvk.Kind]; !seen || preferGVK(gvk, best) {
			preferred[gvk.Kind] = gvk
		}
	}

	out := make(map[string]schema.GroupVersionResource, len(preferred))
	for kind, gvk := range preferred {
		plural, _ := meta.UnsafeGuessKindToResource(gvk)
		out[kind] = plural
	}
	return out
}

// preferGVK reports whether candidate is a better choice than current for the
// same Kind: the kube-aware "more stable / newer" API version wins (GA beats
// beta beats alpha, higher numbers first). Ties break toward the core group,
// then the lexicographically smaller group so the result is deterministic.
func preferGVK(candidate, current schema.GroupVersionKind) bool {
	if cmp := apiversion.CompareKubeAwareVersionStrings(candidate.Version, current.Version); cmp != 0 {
		return cmp > 0
	}
	if candidate.Group == current.Group {
		return false
	}
	if candidate.Group == "" {
		return true
	}
	if current.Group == "" {
		return false
	}
	return candidate.Group < current.Group
}

// kindToGVR maps a Kubernetes kind name to its GroupVersionResource using the
// compiled-in client-go scheme (all built-in kinds, preferred stable version).
// Unknown kinds - including CRDs, which need cluster discovery - return an error.
func kindToGVR(kind string) (schema.GroupVersionResource, error) {
	if gvr, ok := builtinKindToGVR[kind]; ok {
		return gvr, nil
	}
	return schema.GroupVersionResource{}, fmt.Errorf("unknown kind %q; it is not a built-in Kubernetes kind (CRDs are not resolvable without cluster discovery)", kind)
}
