package matrix

import "fmt"

// Case represents a single test case entry.
type Case struct {
	Name  string
	Set   map[string]any // merges into test inputs.set
	Extra map[string]any // everything else, accessible as case.X in CEL
}

// ExpandCases parses the raw cases list from YAML.
// Each case is map[string]any with "name" (required string) and optional "set" (map).
// All other keys go into Extra.
func ExpandCases(raw []map[string]any) ([]Case, error) {
	cases := make([]Case, 0, len(raw))
	for i, entry := range raw {
		nameVal, ok := entry["name"]
		if !ok {
			return nil, fmt.Errorf("cases[%d]: missing required field 'name'", i)
		}
		name, ok := nameVal.(string)
		if !ok {
			return nil, fmt.Errorf("cases[%d]: 'name' must be a string, got %T", i, nameVal)
		}
		if name == "" {
			return nil, fmt.Errorf("cases[%d]: 'name' must not be empty", i)
		}

		var set map[string]any
		if setVal, ok := entry["set"]; ok {
			set, ok = setVal.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("cases[%d] (%q): 'set' must be a map", i, name)
			}
		}

		extra := make(map[string]any)
		for k, v := range entry {
			if k == "name" || k == "set" {
				continue
			}
			extra[k] = v
		}

		cases = append(cases, Case{
			Name:  name,
			Set:   set,
			Extra: extra,
		})
	}
	return cases, nil
}
