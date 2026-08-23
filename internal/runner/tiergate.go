package runner

import (
	"fmt"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/matchers"
)

// matcherTierSkip decides whether a test must be skipped because one of its
// matchers does not support the active backend's tier. The reason names the
// offending matcher, so users see *which* assertion caused the skip, and the
// `--cluster` value that would run it.
func matcherTierSkip(asserts []dsl.Assertion, activeTier string) (skip bool, reason string) {
	name, needTier, ok := matchers.FindUnsupportedMatcher(asserts, activeTier)
	if !ok {
		return false, ""
	}
	return true, fmt.Sprintf("%q is not available at tier %s; run with %s",
		name, activeTier, clusterHintForTier(needTier))
}

// clusterHintForTier names the `--cluster` values that satisfy needTier,
// listing only backends the factory can actually build.
//
// It deliberately does not echo needTier back at the user. Every matcher's
// tier list is a suffix of template < apiserver < simulated < e2e, so needTier
// is the *lowest* tier that would work - and for the waitFor/lookup family
// that is "simulated", a backend which arrives in a later release and which
// `--cluster simulated` currently rejects. Naming a flag value that errors out
// is worse than saying nothing, so the hint points at the backends that both
// exist and provide what the matcher needs.
func clusterHintForTier(needTier string) string {
	if needTier == matchers.TierAPIServer {
		return "--cluster envtest (or kind|k3d|kubeconfig)"
	}
	return "--cluster kind|k3d|kubeconfig"
}
