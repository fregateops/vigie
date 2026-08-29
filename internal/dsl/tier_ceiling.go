package dsl

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// SuiteTier returns the tier a suite declares, defaulting to the strictest one
// so an undeclared file is held to render-only matchers.
func (s *Suite) SuiteTier() string {
	if s.Tier == "" {
		return TierTemplate
	}
	return s.Tier
}

// ValidateTierCeiling reports the first assertion needing more than the suite's
// declared tier provides. The tier is a ceiling on what the file may contain,
// so this is the check that makes an undeclared suite mean "render-only".
func ValidateTierCeiling(suite *Suite) error {
	tier := suite.SuiteTier()
	if !slices.Contains(RunnableTiers(), tier) {
		return fmt.Errorf("tier %q is not a tier vigie can run: use one of %s",
			tier, strings.Join(RunnableTiers(), ", "))
	}
	provided := TierProvides(tier)

	for _, test := range suite.Tests {
		for _, assertion := range test.Asserts {
			name, needs, ok := firstUnmetMatcher(assertion, provided)
			if !ok {
				continue
			}
			return fmt.Errorf(
				"test %q uses %q, which needs %s and so cannot run at tier %s: "+
					"declare `tier: %s` on the suite, or use a matcher that works at %s",
				test.It, name, joinNeeds(needs), tier, lowestRunnableTierFor(needs), tier)
		}
	}
	return nil
}

// firstUnmetMatcher walks one assertion, recursing through composites and
// lookup's nested assertions, and returns the first matcher whose capabilities
// the tier does not provide.
func firstUnmetMatcher(a Assertion, provided map[Capability]bool) (name string, needs []Capability, ok bool) {
	for _, child := range slices.Concat(a.AllOf, a.AnyOf, nestedLookupAsserts(a)) {
		if name, needs, ok = firstUnmetMatcher(child, provided); ok {
			return name, needs, true
		}
	}
	for _, m := range setMatchers(a) {
		for _, need := range matcherCapabilities[m] {
			if !provided[need] {
				return m, matcherCapabilities[m], true
			}
		}
	}
	return "", nil, false
}

func nestedLookupAsserts(a Assertion) []Assertion {
	if a.Lookup == nil {
		return nil
	}
	return slices.Concat(a.Lookup.Then, a.Lookup.ForEach)
}

// setMatchers names the matcher fields this assertion actually sets.
func setMatchers(a Assertion) []string {
	var out []string
	v := reflect.ValueOf(a)
	t := v.Type()
	for i := range t.NumField() {
		name := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		if _, isMatcher := matcherCapabilities[name]; !isMatcher {
			continue
		}
		if !v.Field(i).IsZero() {
			out = append(out, name)
		}
	}
	return out
}

// lowestRunnableTierFor names the cheapest tier a user can actually declare to
// satisfy needs. The lowest tier that provides them may be simulated, which no
// backend reaches yet, so suggesting it would name a value this same validator
// rejects.
func lowestRunnableTierFor(needs []Capability) string {
	runnable := RunnableTiers()
	for _, tier := range TiersProviding(needs) {
		if slices.Contains(runnable, tier) {
			return tier
		}
	}
	return TierE2E
}

func joinNeeds(needs []Capability) string {
	out := make([]string, len(needs))
	for i, c := range needs {
		out[i] = string(c)
	}
	return strings.Join(out, " + ")
}
