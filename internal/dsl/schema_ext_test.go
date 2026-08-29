package dsl

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The published schema is what an editor reads, so a cluster-tier matcher must
// carry its requirement there - otherwise the only way to learn a matcher needs
// a cluster is to run it and read a skip.
func TestSchemaAnnotatesClusterMatchers(t *testing.T) {
	var doc struct {
		Tiers map[string][]string `json:"x-vigie-tiers"`
		Defs  map[string]struct {
			Properties map[string]struct {
				Description  string   `json:"description"`
				Capabilities []string `json:"x-vigie-capabilities"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON(), &doc); err != nil {
		t.Fatalf("parsing embedded schema: %v", err)
	}
	props := doc.Defs["Assertion"].Properties
	if len(props) == 0 {
		t.Fatal("schema has no Assertion properties")
	}

	cases := []struct {
		matcher  string
		caps     []string
		capsPart string
		backend  string
	}{
		{"applies", []string{"apiServerAdmission"}, "apiServerAdmission", "envtest"},
		{"waitFor", []string{"liveReads", "reconciliation"}, "liveReads + reconciliation", "kind"},
		{"http", []string{"runningPods", "networkEgress"}, "runningPods + networkEgress", "kind"},
		{"logsContain", []string{"runningPods", "podLogs"}, "runningPods + podLogs", "kind"},
	}
	for _, tc := range cases {
		t.Run(tc.matcher, func(t *testing.T) {
			p, ok := props[tc.matcher]
			if !ok {
				t.Fatalf("%s missing from the schema", tc.matcher)
			}
			if !reflect.DeepEqual(p.Capabilities, tc.caps) {
				t.Errorf("x-vigie-capabilities = %v, want %v", p.Capabilities, tc.caps)
			}
			if !strings.Contains(p.Description, tc.capsPart) {
				t.Errorf("description %q does not name %q", p.Description, tc.capsPart)
			}
			if !strings.Contains(p.Description, "--cluster "+tc.backend) {
				t.Errorf("description %q does not name a usable backend", p.Description)
			}
			// simulated has no backend the factory can build; never send a
			// reader there.
			if strings.Contains(p.Description, "--cluster simulated") {
				t.Errorf("description %q offers the unimplemented simulated backend", p.Description)
			}
		})
	}

	// A matcher that runs everywhere carries no requirement, so it stays clean.
	if p := props["equal"]; len(p.Capabilities) != 0 || strings.Contains(p.Description, "--cluster") {
		t.Errorf("equal should carry no annotation, got caps=%v desc=%q", p.Capabilities, p.Description)
	}

	// The root mapping is what lets a reader resolve those capability lists to a
	// tier without knowing vigie's internals.
	if doc.Tiers[TierTemplate] == nil {
		t.Fatal("schema publishes no x-vigie-tiers mapping")
	}
	if _, ok := doc.Tiers[TierSimulated]; ok {
		t.Error("x-vigie-tiers advertises simulated, which no --cluster value reaches")
	}
	for _, need := range []string{"runningPods", "podLogs", "networkEgress", "reconciliation"} {
		if !slices.Contains(doc.Tiers[TierE2E], need) {
			t.Errorf("e2e is missing the inherited capability %q", need)
		}
	}
	// Every capability a matcher declares must be provided by some published
	// tier, or nothing could ever run it.
	for _, p := range props {
		for _, need := range p.Capabilities {
			if !slices.Contains(doc.Tiers[TierE2E], need) {
				t.Errorf("capability %q is required by a matcher but no tier provides it", need)
			}
		}
	}
}

func TestBackendsForTier(t *testing.T) {
	if got := BackendsForTier(TierTemplate); got != nil {
		t.Errorf("the template tier needs no cluster, got %v", got)
	}
	for _, tier := range []string{TierAPIServer, TierSimulated, TierE2E} {
		got := BackendsForTier(tier)
		if len(got) == 0 {
			t.Errorf("%s: no backend satisfies it", tier)
		}
		for _, b := range got {
			if b == TierSimulated {
				t.Errorf("%s: offers the unimplemented simulated backend", tier)
			}
		}
	}
	// envtest reaches apiserver but not the tiers above it.
	if got := BackendsForTier(TierE2E); len(got) > 0 && got[0] == "envtest" {
		t.Errorf("envtest cannot satisfy the e2e tier, got %v", got)
	}
}
