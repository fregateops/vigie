package dsl

import (
	"fmt"
	"reflect"
	"strings"
)

// Capability is one thing an environment offers a test. Matchers declare what
// they need, backends declare what they provide, and a tier is a named bundle
// of the two - capabilities being the source of truth, tiers the shorthand.
type Capability string

const (
	RenderedManifests Capability = "renderedManifests"
	// A real API server accepting or rejecting a manifest.
	APIServerAdmission Capability = "apiServerAdmission"
	// GET/LIST against live objects.
	LiveReads Capability = "liveReads"
	// Controllers populating status, conditions and events. envtest has an API
	// server but no controllers, so it stops short of this.
	Reconciliation Capability = "reconciliation"
	// A kubelet actually running containers.
	RunningPods Capability = "runningPods"
	PodLogs     Capability = "podLogs"
	// Reaching a workload by port-forward, ingress or node IP.
	NetworkEgress Capability = "networkEgress"
)

// Tier names, as users write them and `--cluster` maps onto.
const (
	TierTemplate  = "template"
	TierAPIServer = "apiserver"
	TierSimulated = "simulated"
	TierE2E       = "e2e"
)

// AllTiers returns every tier from least to most capable. Each tier provides
// everything the tiers before it do, so a matcher's usable tiers are always a
// suffix of this list.
func AllTiers() []string {
	return []string{TierTemplate, TierAPIServer, TierSimulated, TierE2E}
}

// What each tier adds to the one below it. Increments, not full sets, so the
// cumulative property is structural rather than four lists that must agree.
var tierCapabilities = map[string][]Capability{
	TierTemplate:  {RenderedManifests},
	TierAPIServer: {APIServerAdmission, LiveReads},
	TierSimulated: {Reconciliation},
	TierE2E:       {RunningPods, PodLogs, NetworkEgress},
}

// TierProvides reports the full capability set of a tier, including everything
// inherited from the tiers below it.
func TierProvides(tier string) map[Capability]bool {
	provided := make(map[Capability]bool)
	for _, t := range AllTiers() {
		for _, c := range tierCapabilities[t] {
			provided[c] = true
		}
		if t == tier {
			return provided
		}
	}
	return nil // unknown tier provides nothing, so it fails closed
}

// TiersProviding returns the tiers whose capability set covers needs, in ladder
// order. An empty needs list is satisfied everywhere.
func TiersProviding(needs []Capability) []string {
	var out []string
	for _, tier := range AllTiers() {
		provided := TierProvides(tier)
		satisfied := true
		for _, need := range needs {
			if !provided[need] {
				satisfied = false
				break
			}
		}
		if satisfied {
			out = append(out, tier)
		}
	}
	return out
}

// Matcher YAML key -> capabilities, read once from the tags on Assertion.
var matcherCapabilities = buildMatcherCapabilities()

// CapabilitiesForMatcher returns the capabilities the named matcher needs.
func CapabilitiesForMatcher(name string) ([]Capability, bool) {
	caps, ok := matcherCapabilities[name]
	return caps, ok
}

// TiersForMatcher returns the tiers the named matcher can run at. An unknown
// name is a programmer error, so it panics rather than guess a permissive
// default.
func TiersForMatcher(name string) []string {
	caps, ok := matcherCapabilities[name]
	if !ok {
		panic(fmt.Sprintf("dsl: no capabilities registered for matcher %q", name))
	}
	return TiersProviding(caps)
}

// MatcherNames lists every matcher field declared on Assertion.
func MatcherNames() []string {
	names := make([]string, 0, len(matcherCapabilities))
	for name := range matcherCapabilities {
		names = append(names, name)
	}
	return names
}

// buildMatcherCapabilities reads the `vigie` tag off every Assertion field:
// matchers declare `needs=...`, structural fields declare `-`. An untagged
// field panics at init - a matcher with unknown requirements would otherwise
// run in the template tier and produce a wrong result instead of a skip.
func buildMatcherCapabilities() map[string][]Capability {
	out := make(map[string][]Capability)
	t := reflect.TypeOf(Assertion{})
	for i := range t.NumField() {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		tag, ok := field.Tag.Lookup("vigie")
		if !ok {
			panic(fmt.Sprintf(
				"dsl: Assertion.%s has no `vigie` tag: every matcher field must declare "+
					"`vigie:\"needs=...\"`, and structural fields must declare `vigie:\"-\"`",
				field.Name))
		}
		if tag == "-" {
			continue
		}
		caps, err := parseNeedsTag(tag)
		if err != nil {
			panic(fmt.Sprintf("dsl: Assertion.%s: %v", field.Name, err))
		}
		out[name] = caps
	}
	return out
}

// parseNeedsTag parses `needs=a,b,c`, rejecting unknown capability names.
func parseNeedsTag(tag string) ([]Capability, error) {
	spec, ok := strings.CutPrefix(tag, "needs=")
	if !ok {
		return nil, fmt.Errorf("vigie tag %q is not `needs=...`", tag)
	}
	var caps []Capability
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c := Capability(part)
		if !knownCapabilities[c] {
			return nil, fmt.Errorf("unknown capability %q", part)
		}
		caps = append(caps, c)
	}
	if len(caps) == 0 {
		return nil, fmt.Errorf("vigie tag %q declares no capabilities", tag)
	}
	return caps, nil
}

var knownCapabilities = map[Capability]bool{
	RenderedManifests:  true,
	APIServerAdmission: true,
	LiveReads:          true,
	Reconciliation:     true,
	RunningPods:        true,
	PodLogs:            true,
	NetworkEgress:      true,
}
