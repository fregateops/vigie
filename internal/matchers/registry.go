package matchers

import (
	"fmt"

	"github.com/fregateops/vigie/internal/dsl"
)

// registry holds every Matcher implementation. Entries are appended by
// Register, which is called from each matcher file's init(). Iteration order
// follows registration order, which makes the legacy switch's first-case-wins
// semantics predictable when two Matches predicates could both be true for a
// constructed assertion (e.g. someone setting both a.Equal and a.AllOf).
var registry []Matcher

// Register adds m to the dispatch registry. Panics on duplicate Name() so a
// programming error surfaces at init time instead of as a silent dispatch
// surprise at runtime.
func Register(m Matcher) {
	for _, existing := range registry {
		if existing.Name() == m.Name() {
			panic(fmt.Sprintf("matchers: duplicate registration for %q", m.Name()))
		}
	}
	registry = append(registry, m)
}

// find returns the first registered matcher whose Matches reports true for
// a. ok=false means no registered matcher claims the assertion — Evaluate
// surfaces that as the "unknown or unsupported matcher" diagnostic.
func find(a dsl.Assertion) (Matcher, bool) {
	for _, m := range registry {
		if m.Matches(a) {
			return m, true
		}
	}
	return nil, false
}

// registeredNames returns the canonical name of every registered matcher in
// registration order. Used to build the "unknown matcher" diagnostic so the
// list stays in sync with the registry instead of drifting in a hardcoded
// string.
func registeredNames() []string {
	out := make([]string, len(registry))
	for idx, m := range registry {
		out[idx] = m.Name()
	}
	return out
}
