package matchers

import "github.com/fregateops/vigie/internal/dsl"

// Aliases onto internal/dsl, which owns the tier names alongside the
// capability tags that define them.
const (
	TierTemplate  = dsl.TierTemplate
	TierAPIServer = dsl.TierAPIServer
	TierSimulated = dsl.TierSimulated
	TierE2E       = dsl.TierE2E
)

// TierInList reports whether active appears in the supported list. The
// list-based model replaces the rank-based TierSatisfies — every matcher
// enumerates the tiers it works on, and the runner asks "is the active
// label among them?".
func TierInList(active string, supported []string) bool {
	for _, tier := range supported {
		if tier == active {
			return true
		}
	}
	return false
}

// SupportedTiersFor returns the tiers under which the assertion can run,
// derived from the capabilities its matcher declares in internal/dsl.
//
// An assertion no matcher claims needs everything, so it runs only at the most
// capable tier: failing closed, where granting every tier would let a forgotten
// registry entry produce a wrong result instead of a skip.
func SupportedTiersFor(a dsl.Assertion) []string {
	if m, ok := find(a); ok {
		return m.SupportedTiers(a)
	}
	return []string{TierE2E}
}

// intersectTiers returns the slice of labels common to a and b, preserving
// the order they appear in a. The order matters: FindUnsupportedMatcher
// reports the "needed tier" as the first surviving label, which we want to
// be the strictest still-accepted tier (a's caller passes the matcher's own
// tier list as a, so its order is authoritative).
func intersectTiers(a, b []string) []string {
	out := make([]string, 0, len(a))
	for _, label := range a {
		for _, other := range b {
			if label == other {
				out = append(out, label)
				break
			}
		}
	}
	return out
}

// intersectChildTiers returns the tier set common to every child assertion.
// Used by composite matchers (allOf, anyOf) so the runner satisfies the
// strictest child. anyOf uses intersection too — see quantifier.go for why
// we don't treat anyOf as a union here.
func intersectChildTiers(children []dsl.Assertion) []string {
	if len(children) == 0 {
		return dsl.AllTiers()
	}
	result := SupportedTiersFor(children[0])
	for _, child := range children[1:] {
		result = intersectTiers(result, SupportedTiersFor(child))
	}
	return result
}

// FindUnsupportedMatcher walks asserts (recursively, through allOf/anyOf) and
// returns the first assertion whose matcher excludes activeTier. When ok is
// true, the caller should skip the test with a message naming `name` and the
// `neededTier` — the strictest tier still in the matcher's supported list.
//
// `neededTier` lets the runner phrase the skip as "requires tier <X>"
// without exposing the full supported slice. For a matcher that supports
// `[simulated, e2e]` on an apiserver backend, neededTier is "simulated".
func FindUnsupportedMatcher(asserts []dsl.Assertion, activeTier string) (name, neededTier string, ok bool) {
	for _, assertion := range asserts {
		if name, neededTier, ok = findUnsupportedInAssertion(assertion, activeTier); ok {
			return name, neededTier, true
		}
	}
	return "", "", false
}

// findUnsupportedInAssertion handles one assertion. Composite matchers
// recurse into their children directly — that's a more accurate report than
// blaming the composite itself for a tier mismatch.
func findUnsupportedInAssertion(a dsl.Assertion, activeTier string) (name, neededTier string, ok bool) {
	if len(a.AllOf) > 0 {
		return FindUnsupportedMatcher(a.AllOf, activeTier)
	}
	if len(a.AnyOf) > 0 {
		return FindUnsupportedMatcher(a.AnyOf, activeTier)
	}
	matcher, found := find(a)
	if !found {
		return "", "", false
	}
	supported := matcher.SupportedTiers(a)
	if TierInList(activeTier, supported) {
		return "", "", false
	}
	return matcher.Name(), strictestTier(supported), true
}

// strictestTier returns the lowest-rank tier still present in supported —
// i.e. the easiest tier on which the matcher will actually run, which is
// also the threshold the user needs to reach. For `[simulated, e2e]` it
// returns "simulated"; for `[e2e]` it returns "e2e". Empty list yields ""
// (matcher claims it runs nowhere — caller treats that as a hard skip).
func strictestTier(supported []string) string {
	const noRank = -1
	bestRank := noRank
	chosen := ""
	for _, tier := range supported {
		rank := tierRank(tier)
		if bestRank == noRank || rank < bestRank {
			bestRank = rank
			chosen = tier
		}
	}
	return chosen
}

// tierRank orders tier labels from least to most capable. Used only inside
// strictestTier — the public API works in terms of explicit lists, not
// ranks. Unknown labels rank as TierTemplate so a future bogus value never
// hides a matcher's real requirement.
func tierRank(tier string) int {
	switch tier {
	case TierE2E:
		return 3
	case TierSimulated:
		return 2
	case TierAPIServer:
		return 1
	default:
		return 0
	}
}
