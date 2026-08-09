package matchers

import "github.com/fregateops/vigie/internal/dsl"

// Tier names. These match the values produced by runner.backendTier and the
// values users write under `tier:` in test files. Kept as constants so every
// reference can be checked at compile time.
const (
	TierTemplate  = "template"
	TierAPIServer = "apiserver"
	TierSimulated = "simulated"
	TierE2E       = "e2e"
)

// AllTiers is the supported-tier list returned by the majority of matchers —
// every assertion that doesn't need a live cluster (equal, exists, contains,
// matchRegex, isType, matchSnapshot, matchSchema, expr, …) returns this.
// Defined as a package-level value so 31 matcher files share one literal
// instead of each rebuilding the slice.
//
// IMPORTANT: never mutate. The Tiers helper exists for callers who need a
// fresh slice; AllTiers is read-only by convention.
var AllTiers = []string{TierTemplate, TierAPIServer, TierSimulated, TierE2E}

// Tiers builds a defensive-copied list. Used by matchers that support a
// proper subset of tiers (applies, waitFor, http, …) so the per-matcher
// SupportedTiers method can return a fresh slice the caller can mutate
// safely.
func Tiers(t ...string) []string { return append([]string(nil), t...) }

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

// SupportedTiersFor returns the list of tiers under which the assertion can
// run. Looks up the matcher in the registry and asks it. Unknown matchers
// (no Matches predicate claims the assertion) get AllTiers as a charitable
// default — the runner already surfaces unknown matchers via Evaluate's
// fallback message, so the tier gate doesn't need to fire too.
func SupportedTiersFor(a dsl.Assertion) []string {
	if m, ok := find(a); ok {
		return m.SupportedTiers(a)
	}
	return Tiers(AllTiers...)
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
		return Tiers(AllTiers...)
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
