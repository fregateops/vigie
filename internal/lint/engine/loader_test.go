package engine

import "testing"

func TestLoadAll_BuiltinRules(t *testing.T) {
	rules, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	sets := map[string]int{}
	for _, r := range rules {
		sets[r.SetName()]++
	}
	for _, want := range []string{"chart-yaml", "template-best-practices", "deprecation"} {
		if sets[want] == 0 {
			t.Errorf("rule set %q has no rules", want)
		}
	}
}
