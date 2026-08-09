package matrix

import "fmt"

type dim struct {
	key    string
	values []any
}

// Expand computes the Cartesian product of dimension lists.
// The input is the raw matrix map from YAML (includes "exclude" and "include" special keys).
// Dimensions are all keys except "exclude" and "include".
// Each dimension value is []any.
// exclude: []map[string]any - combos to drop (match if all specified keys match)
// include: []map[string]any - combos to inject even if excluded (or new combos)
// Returns slice of maps, one per expanded entry.
func Expand(raw map[string]any) ([]map[string]any, error) {
	// Extract special keys.
	var excludes []map[string]any
	if v, ok := raw["exclude"]; ok {
		excludes, ok = toSliceOfMaps(v)
		if !ok {
			return nil, fmt.Errorf("matrix: 'exclude' must be a list of maps")
		}
	}

	var includes []map[string]any
	if v, ok := raw["include"]; ok {
		includes, ok = toSliceOfMaps(v)
		if !ok {
			return nil, fmt.Errorf("matrix: 'include' must be a list of maps")
		}
	}

	// Build ordered dimension keys and values (all keys except exclude/include).
	var dims []dim
	for k, v := range raw {
		if k == "exclude" || k == "include" {
			continue
		}
		vals, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("matrix: dimension %q must be a list", k)
		}
		dims = append(dims, dim{key: k, values: vals})
	}

	// Compute Cartesian product.
	product := cartesian(dims)

	// Filter out excluded combos.
	filtered := make([]map[string]any, 0, len(product))
	for _, combo := range product {
		if !matchesAnyExclude(combo, excludes) {
			filtered = append(filtered, combo)
		}
	}

	// Append include entries. Per GitHub Actions semantics: if an include
	// combo is a subset of an existing filtered combo, it is merged into it;
	// otherwise it is appended as a standalone entry.
	for _, inc := range includes {
		merged := false
		for _, combo := range filtered {
			if isSubset(inc, combo) {
				for k, v := range inc {
					combo[k] = v
				}
				merged = true
			}
		}
		if !merged {
			entry := make(map[string]any, len(inc))
			for k, v := range inc {
				entry[k] = v
			}
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// cartesian returns the Cartesian product of the given dimensions.
// Each result is a fresh map.
func cartesian(dims []dim) []map[string]any {
	if len(dims) == 0 {
		return nil
	}

	result := []map[string]any{{}}
	for _, d := range dims {
		var next []map[string]any
		for _, existing := range result {
			for _, val := range d.values {
				entry := make(map[string]any, len(existing)+1)
				for k, v := range existing {
					entry[k] = v
				}
				entry[d.key] = val
				next = append(next, entry)
			}
		}
		result = next
	}
	return result
}

// matchesAnyExclude returns true if combo matches any of the exclude rules.
// A combo matches a rule if every key in the rule has the same value in combo.
func matchesAnyExclude(combo map[string]any, excludes []map[string]any) bool {
	for _, rule := range excludes {
		if matchesExclude(combo, rule) {
			return true
		}
	}
	return false
}

func matchesExclude(combo, rule map[string]any) bool {
	for k, v := range rule {
		cv, ok := combo[k]
		if !ok {
			return false
		}
		if cv != v {
			return false
		}
	}
	return true
}

// isSubset returns true if every key in sub has the same value in super.
func isSubset(sub, super map[string]any) bool {
	for k, v := range sub {
		sv, ok := super[k]
		if !ok {
			return false
		}
		if sv != v {
			return false
		}
	}
	return true
}

func toSliceOfMaps(v any) ([]map[string]any, bool) {
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}
