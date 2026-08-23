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

// TierForBackend returns the tier a `--cluster` value runs at. An empty value
// or "none" is the in-process template tier; every other backend maps through
// backendTier.
func TierForBackend(backendType string) string {
	if backendType == "" || backendType == "none" {
		return matchers.TierTemplate
	}
	return backendTier(backendType)
}

// UnrunnableAt reports whether every test in a parsed suite would be skipped at
// the given tier, and why. It answers "this file cannot run here at all",
// which callers use to refuse an explicitly requested file instead of running
// it to a green zero-assertion finish. A suite with no tests is not unrunnable
// — that is a different warning.
func UnrunnableAt(suite *dsl.Suite, tier string) (unrunnable bool, reason string) {
	if len(suite.Tests) == 0 {
		return false, ""
	}
	for _, test := range suite.Tests {
		skip, why := matcherTierSkip(test.Asserts, tier)
		if !skip {
			return false, ""
		}
		if reason == "" {
			reason = why
		}
	}
	return true, reason
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
