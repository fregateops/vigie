package runner

import (
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/matchers"
)

func TestMatcherTierSkip_NamesTheMatcherAndAReachableFlag(t *testing.T) {
	cases := []struct {
		name       string
		asserts    []dsl.Assertion
		activeTier string
		wantSkip   bool
		wantParts  []string
	}{
		{
			name:       "apiserver matcher at template tier",
			asserts:    []dsl.Assertion{{Applies: &dsl.AppliesSpec{}}},
			activeTier: matchers.TierTemplate,
			wantSkip:   true,
			wantParts:  []string{`"applies"`, "tier template", "--cluster envtest"},
		},
		{
			name:       "e2e matcher at apiserver tier",
			asserts:    []dsl.Assertion{{HTTP: &dsl.HTTPAssert{}}},
			activeTier: matchers.TierAPIServer,
			wantSkip:   true,
			wantParts:  []string{`"http"`, "tier apiserver", "--cluster kind|k3d|kubeconfig"},
		},
		{
			name:       "template matcher runs everywhere",
			asserts:    []dsl.Assertion{{Equal: &dsl.PathValue{Path: "kind"}}},
			activeTier: matchers.TierTemplate,
			wantSkip:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			skip, reason := matcherTierSkip(tc.asserts, tc.activeTier)
			if skip != tc.wantSkip {
				t.Fatalf("matcherTierSkip skip = %v, want %v (reason %q)", skip, tc.wantSkip, reason)
			}
			for _, want := range tc.wantParts {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q does not contain %q", reason, want)
				}
			}
		})
	}
}

// The simulated backend is not implemented yet, so `--cluster simulated` errors
// out. A skip reason must never send the user there: for the waitFor/lookup
// family the lowest usable tier IS simulated, so the hint has to name the e2e
// backends that exist and also satisfy those matchers.
func TestClusterHintForTier_NeverNamesAnUnreachableBackend(t *testing.T) {
	for _, tier := range matchers.AllTiers {
		hint := clusterHintForTier(tier)
		if strings.Contains(hint, matchers.TierSimulated) {
			t.Errorf("hint for tier %q offers the unimplemented simulated backend: %q", tier, hint)
		}
		if !strings.Contains(hint, "--cluster") {
			t.Errorf("hint for tier %q names no flag to pass: %q", tier, hint)
		}
	}
}

func TestTierForBackend(t *testing.T) {
	cases := map[string]string{
		"":           matchers.TierTemplate,
		"none":       matchers.TierTemplate,
		"envtest":    matchers.TierAPIServer,
		"kind":       matchers.TierE2E,
		"k3d":        matchers.TierE2E,
		"kubeconfig": matchers.TierE2E,
	}
	for backend, want := range cases {
		if got := TierForBackend(backend); got != want {
			t.Errorf("TierForBackend(%q) = %q, want %q", backend, got, want)
		}
	}
}

func TestUnrunnableAt(t *testing.T) {
	clusterTest := dsl.Test{It: "applies", Asserts: []dsl.Assertion{{Applies: &dsl.AppliesSpec{}}}}
	renderTest := dsl.Test{It: "renders", Asserts: []dsl.Assertion{{Equal: &dsl.PathValue{Path: "kind"}}}}

	cases := []struct {
		name           string
		suite          dsl.Suite
		tier           string
		wantUnrunnable bool
	}{
		{
			name:           "every test needs a cluster",
			suite:          dsl.Suite{Tests: []dsl.Test{clusterTest, clusterTest}},
			tier:           matchers.TierTemplate,
			wantUnrunnable: true,
		},
		{
			// One runnable test is enough: the file has something to say here,
			// so refusing it would be wrong.
			name:           "one test can still run",
			suite:          dsl.Suite{Tests: []dsl.Test{clusterTest, renderTest}},
			tier:           matchers.TierTemplate,
			wantUnrunnable: false,
		},
		{
			name:           "the tier satisfies every test",
			suite:          dsl.Suite{Tests: []dsl.Test{clusterTest, renderTest}},
			tier:           matchers.TierE2E,
			wantUnrunnable: false,
		},
		{
			// An empty suite is a different problem, reported as its own warning.
			name:           "no tests at all",
			suite:          dsl.Suite{},
			tier:           matchers.TierTemplate,
			wantUnrunnable: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unrunnable, reason := UnrunnableAt(&tc.suite, tc.tier)
			if unrunnable != tc.wantUnrunnable {
				t.Fatalf("UnrunnableAt = %v, want %v (reason %q)", unrunnable, tc.wantUnrunnable, reason)
			}
			if unrunnable && reason == "" {
				t.Error("an unrunnable suite must explain why")
			}
		})
	}
}

func TestMatcherTierSkip_ReportsTheOffendingMatcherInsideAComposite(t *testing.T) {
	// A composite's tier is the intersection of its children, so a cluster-only
	// matcher nested in allOf must still be named - blaming "allOf" would leave
	// the user hunting for the real cause.
	asserts := []dsl.Assertion{{
		AllOf: []dsl.Assertion{
			{Equal: &dsl.PathValue{Path: "kind"}},
			{LogsContain: &dsl.LogsAssert{}},
		},
	}}
	skip, reason := matcherTierSkip(asserts, matchers.TierTemplate)
	if !skip {
		t.Fatal("a logsContain nested in allOf must skip at the template tier")
	}
	if !strings.Contains(reason, `"logsContain"`) {
		t.Errorf("reason %q does not name the nested matcher", reason)
	}
}
