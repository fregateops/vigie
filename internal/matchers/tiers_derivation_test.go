package matchers

import (
	"reflect"
	"sort"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

// allTiers is captured once so the table below reads as data.
var allTiers = dsl.AllTiers()

// The tier list every matcher had when the data was hand-written in the
// registry, captured verbatim. Deriving from capabilities must reproduce it
// exactly - this is a refactor, not a behaviour change.
var wantTiers = map[string][]string{
	// renderedManifests - no cluster needed.
	"contains": allTiers, "endsWith": allTiers, "equal": allTiers, "exists": allTiers,
	"expr": allTiers, "failedTemplate": allTiers, "greaterThan": allTiers, "gte": allTiers,
	"hasDocuments": allTiers, "isAPIVersion": allTiers, "isEmpty": allTiers, "isKind": allTiers,
	"isNotEmpty": allTiers, "isNotNull": allTiers, "isNull": allTiers, "isSubset": allTiers,
	"isType": allTiers, "lengthEqual": allTiers, "lessThan": allTiers, "lte": allTiers,
	"matchRegex": allTiers, "matchSchema": allTiers, "matchSnapshot": allTiers,
	"matchTemplate": allTiers, "notContains": allTiers, "notEqual": allTiers,
	"notExists": allTiers, "notMatchRegex": allTiers, "startsWith": allTiers,

	// apiServerAdmission - a real API server admits or rejects the manifest.
	"applies":  {TierAPIServer, TierSimulated, TierE2E},
	"rejected": {TierAPIServer, TierSimulated, TierE2E},

	// liveReads + reconciliation - status and conditions must be populated.
	"becomesReady": {TierSimulated, TierE2E},
	"eventEmitted": {TierSimulated, TierE2E},
	"lookup":       {TierSimulated, TierE2E},
	"waitFor":      {TierSimulated, TierE2E},

	// runningPods plus logs or network - a kubelet must be running containers.
	"http":        {TierE2E},
	"logsContain": {TierE2E},
}

func TestDerivedTiersMatchTheHandWrittenTable(t *testing.T) {
	for name, want := range wantTiers {
		got := dsl.TiersForMatcher(name)
		if !reflect.DeepEqual(got, want) {
			caps, _ := dsl.CapabilitiesForMatcher(name)
			t.Errorf("%s: derived tiers %v, want %v (declares needs=%v)", name, got, want, caps)
		}
	}
}

// A tagged field with no registry entry parses but cannot evaluate; a registry
// entry with no field is dead code.
func TestRegistryAndTagsAgree(t *testing.T) {
	tagged := dsl.MatcherNames()
	sort.Strings(tagged)

	registered := make([]string, 0, len(registry))
	for _, m := range registry {
		// Composites derive their tiers by intersecting children, so they carry
		// no capability tag of their own.
		if m.Name() == "allOf" || m.Name() == "anyOf" {
			continue
		}
		registered = append(registered, m.Name())
	}
	sort.Strings(registered)

	if !reflect.DeepEqual(tagged, registered) {
		t.Errorf("matcher sets differ:\n  tagged on Assertion: %v\n  registered:          %v", tagged, registered)
	}
	if len(tagged) != len(wantTiers) {
		t.Errorf("wantTiers covers %d matchers, but %d are tagged", len(wantTiers), len(tagged))
	}
}

// A contiguous suffix is what makes "the tier a file needs" a single floor
// rather than an arbitrary set. Several design decisions rest on it.
func TestEveryMatcherTierListIsASuffixOfTheLadder(t *testing.T) {
	for _, name := range dsl.MatcherNames() {
		got := dsl.TiersForMatcher(name)
		if len(got) == 0 {
			t.Errorf("%s: runs at no tier at all", name)
			continue
		}
		want := allTiers[len(allTiers)-len(got):]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: tiers %v are not a suffix of %v", name, got, allTiers)
		}
	}
}
