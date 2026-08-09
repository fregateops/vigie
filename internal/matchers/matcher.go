package matchers

import "github.com/fregateops/vigie/internal/dsl"

// Matcher is the contract every assertion implementation satisfies. Concrete
// matchers live next to their evalXxx helper (e.g. equalMatcher in equal.go)
// and self-register from init() so the dispatcher in Evaluate and the tier
// gate in FindUnsupportedMatcher both walk the same set.
//
// The interface intentionally takes the whole dsl.Assertion (not the typed
// spec pointer) because Go interfaces cannot vary a method's argument type
// per implementer. Each Evaluate method's first line unwraps its own field
// and delegates to the existing typed evalXxx(spec, ctx) helper, so type
// safety is preserved at that internal boundary.
type Matcher interface {
	// Name returns the matcher's canonical identifier — used in skip-message
	// strings ("logsContain matcher requires tier e2e…"), debug logs, and
	// the unknown-matcher diagnostic.
	Name() string

	// Matches reports whether this matcher is the implementation responsible
	// for the given assertion. The registry walks matchers in registration
	// order and picks the first one whose Matches returns true, mirroring
	// the first-case-wins semantic of the legacy switch.
	Matches(a dsl.Assertion) bool

	// SupportedTiers returns the list of tier labels under which this
	// matcher can produce a meaningful result. Most matchers ignore the
	// assertion argument and return a static list (AllTiers for template-
	// tier matchers, or Tiers(...) for cluster-tier matchers). Composite
	// matchers (allOf, anyOf) recurse into children to intersect their
	// supported sets — that's why the method takes the assertion at all.
	SupportedTiers(a dsl.Assertion) []string

	// Evaluate runs the matcher against the assertion and eval context and
	// returns the Result. Negation (a.Not) is applied by the dispatcher
	// after Evaluate returns, so implementations should not check it.
	Evaluate(a dsl.Assertion, ctx EvalContext) Result
}

// simpleMatcher implements Matcher for assertions whose supported tiers are
// static. All non-quantifier matchers use this; allOfMatcher/anyOfMatcher
// remain named types because their SupportedTiers recurse into children.
type simpleMatcher struct {
	name     string
	matches  func(dsl.Assertion) bool
	tiers    []string
	evaluate func(dsl.Assertion, EvalContext) Result
}

func (m simpleMatcher) Name() string                            { return m.name }
func (m simpleMatcher) Matches(a dsl.Assertion) bool            { return m.matches(a) }
func (m simpleMatcher) SupportedTiers(_ dsl.Assertion) []string { return m.tiers }
func (m simpleMatcher) Evaluate(a dsl.Assertion, ctx EvalContext) Result {
	return m.evaluate(a, ctx)
}
