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

// integrationFeatureSkip reports whether a suite declares integration-tier
// features - dependencies, or lifecycle hooks at suite or test scope - that the
// active tier cannot provide, and why.
//
// These used to be dropped with a log warning while the tests ran on, on the
// theory that assertions might still pass without them. They can, and that is
// exactly the problem: a suite whose staged dependencies were never installed
// reports green having verified a premise that never held. The suite's tests
// are skipped instead, so the gap is visible and counted.
func integrationFeatureSkip(suite *dsl.Suite, tier string) (skip bool, reason string) {
	if tierAtLeast(tier, matchers.TierSimulated) {
		return false, ""
	}
	hint := clusterHintForTier(matchers.TierSimulated)
	switch {
	case len(suite.Dependencies) > 0:
		return true, fmt.Sprintf("suite declares dependencies, which tier %s cannot install; run with %s", tier, hint)
	case len(suite.BeforeAll)+len(suite.AfterAll) > 0:
		return true, fmt.Sprintf("suite declares beforeAll/afterAll hooks, which tier %s cannot run; run with %s", tier, hint)
	case suiteHasTestHooks(suite):
		return true, fmt.Sprintf("suite declares setup/teardown hooks, which tier %s cannot run; run with %s", tier, hint)
	}
	return false, ""
}

func suiteHasTestHooks(suite *dsl.Suite) bool {
	for _, test := range suite.Tests {
		if len(test.Setup)+len(test.Teardown) > 0 {
			return true
		}
	}
	return false
}

// tierAtLeast reports whether active sits at or above want on the ladder. An
// unrecognised tier ranks below everything, so it is treated as providing
// nothing rather than as satisfying a requirement by accident.
func tierAtLeast(active, want string) bool {
	return tierRank(active) >= tierRank(want)
}

func tierRank(tier string) int {
	for i, t := range matchers.AllTiers {
		if t == tier {
			return i
		}
	}
	return -1
}

// UnrunnableAt reports whether every test in a parsed suite would be skipped at
// the given tier, and why. It answers "this file cannot run here at all",
// which callers use to refuse an explicitly requested file instead of running
// it to a green zero-assertion finish. A suite with no tests is not unrunnable
// — that is a different warning.
func UnrunnableAt(suite *dsl.Suite, tier string) (unrunnable bool, reason string) {
	if skip, reason := integrationFeatureSkip(suite, tier); skip {
		return true, reason
	}
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
